package deferredidentity

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2016"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2016",
	Doc: `A2016: identity established in a message handler that Init sent to itself.

Init is where a process becomes reachable. RegisterName is what makes peers able to
address it, RegisterEvent is what makes subscribers able to link, and CreateAlias is
what makes a token usable. Doing any of them one mailbox pass later is not the same
thing done slightly later: Spawn has already returned the PID to the parent, the
parent has already told whoever was waiting, and until the deferred message is handled
a peer addressing the name gets ErrProcessUnknown and a subscriber gets
ErrEventUnknown. Nothing retries those.

It also splits an invariant. The token RegisterEvent returns is assigned in one
callback and consumed in another, so publishing is correct only because the mailbox
happened to deliver the setup message before the first thing that publishes. That is
an ordering assumption no signature states and no test that starts by sending the
setup message can fail.

The reported shape is the deferred one specifically, not any late registration: Init
sends a message to this same process, and a message handler of the same behavior
performs the identity call. A process that renames itself on an operator command, or
registers an event when the first consumer asks, is doing something dynamic on
purpose, has no Init self-send, and stays silent.

Self recognition is A1003's: the process PID and its registered name, through the
accessors, both route back into this mailbox.

The fix is to do it in Init. If the work Init deferred is genuinely slow, defer the
slow part and keep the identity where it belongs.

Source: actors.md, events.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2016",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var identityCalls = map[string]string{
	"RegisterName":  "a registered name",
	"RegisterEvent": "an event",
	"CreateAlias":   "an alias",
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	deferred := map[*types.Named]token.Pos{}
	for _, cb := range m.Callbacks {
		if cb.Name != "Init" && cb.Name != "ProcessInit" {
			continue
		}
		for _, site := range m.SendSites {
			if site.In != cb || len(site.Call.Args) == 0 {
				continue
			}
			if selfTarget(pass, cb.Recv, site.Call.Args[0]) == false {
				continue
			}
			deferred[cb.Behavior] = site.Call.Pos()
			break
		}
	}

	for _, cb := range m.Callbacks {
		if handlesMessages(cb.Name) == false {
			continue
		}
		at, isDeferred := deferred[cb.Behavior]
		if isDeferred == false {
			continue
		}
		for _, call := range identitySetup(pass, cb.Decl) {
			m.Report(pass, call.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: call.method,
				},
				"%s establishes %s, and Init deferred to this handler with the self send at line %d: Spawn has already returned this process to its parent, so until this message is handled a peer addressing it gets gen.ErrProcessUnknown and a subscriber gets gen.ErrEventUnknown, and nothing retries either. Call %s in Init and defer only the slow work",
				cb.Name, identityCalls[call.method], pass.Fset.Position(at).Line, call.method)
		}
	}
	return nil, nil
}

type setup struct {
	method string
	pos    token.Pos
}

func identitySetup(pass *analysis.Pass, decl *ast.FuncDecl) []setup {
	if decl.Body == nil {
		return nil
	}
	var out []setup
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if isSel == false || identityCalls[sel.Sel.Name] == "" {
			return true
		}
		fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		if isFunc == false || fn.Pkg() == nil {
			return true
		}
		if strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/") == false {
			return true
		}
		out = append(out, setup{method: sel.Sel.Name, pos: call.Pos()})
		return true
	}
	ast.Inspect(decl.Body, walk)
	return out
}

func handlesMessages(name string) bool {
	switch name {
	case "HandleMessage", "HandleMessageName", "HandleMessageAlias":
		return true
	}
	return false
}

func selfTarget(pass *analysis.Pass, recv string, e ast.Expr) bool {
	if recv == "" {
		return false
	}
	call, ok := e.(*ast.CallExpr)
	if ok == false {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false {
		return false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if isIdent == false || id.Name != recv {
		return false
	}
	switch sel.Sel.Name {
	case "PID", "Name":
		fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		return isFunc && fn.Pkg() != nil &&
			strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/")
	}
	return false
}

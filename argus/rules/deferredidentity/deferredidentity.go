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

Init is where a process becomes reachable. The message Init sends to itself is handled
only after Init has returned, and by then Spawn has handed the PID to the parent, which
may already have told whoever was waiting. Until the deferred message is handled, what
the handler is about to establish does not exist, and each kind fails in its own way:

  - RegisterName: nothing is registered under the name, so a send or a call addressed
    to it fails with gen.ErrProcessUnknown. A process that must be reachable by name
    from the start registers it in Init, or is named at spawn with SpawnRegister, which
    registers the name before Init runs; such a process already holds a name, and a
    RegisterName on it returns gen.ErrTaken wherever it is called.
  - RegisterEvent: the event is unknown, so LinkEvent and MonitorEvent on it fail with
    gen.ErrEventUnknown.
  - CreateAlias: nothing outside can fail, since nobody can address an alias before
    the process hands it out. What is left is the split invariant below, so a deferred
    alias is a tier 3 hint, not a defect.

It also splits an invariant. The token RegisterEvent returns, or the alias, is assigned
in one callback and consumed in another, so the code that uses it is correct only
because the setup message is handled before anything that reads it. The self send is
first in the main queue, but a message sent with a higher priority after Spawn returns
is handled ahead of it. That is an ordering assumption no signature states and no test
that starts by sending the setup message can fail.

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

var identityCalls = map[string]struct {
	tier    int
	message string
}{
	"RegisterName":  {tier: 2, message: "%s registers a name, deferred by the self send in Init at line %d: Spawn has handed this process to its parent before this message is handled, and until then a send or call addressed to the name fails with gen.ErrProcessUnknown. Register it in Init, or name the process at spawn with SpawnRegister, after which RegisterName returns gen.ErrTaken"},
	"RegisterEvent": {tier: 2, message: "%s registers an event, deferred by the self send in Init at line %d: Spawn has handed this process to its parent before this message is handled, and until then LinkEvent and MonitorEvent on the event fail with gen.ErrEventUnknown. Call RegisterEvent in Init and defer only the slow work"},
	"CreateAlias":   {tier: 3, message: "%s creates an alias, deferred by the self send in Init at line %d: nobody can address the alias before it exists, so nothing outside fails, but a callback that reads it is correct only while this message is handled first. Call CreateAlias in Init"},
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
			kind := identityCalls[call.method]
			m.Report(pass, call.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: kind.tier,
					ID: ergomodel.CallbackID(cb), Witness: call.method,
				},
				kind.message, cb.Name, pass.Fset.Position(at).Line)
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
		if isSel == false {
			return true
		}
		if _, identity := identityCalls[sel.Sel.Name]; identity == false {
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

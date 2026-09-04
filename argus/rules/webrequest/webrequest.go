package webrequest

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2008"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2008",
	Doc: `A2008: a web request that is never completed, or handled where it never arrives.

The web handler meta does not hand over a request and walk away. ServeHTTP builds the
message with Done set to the context cancel, sends it to the worker, and then blocks on
the context: the HTTP goroutine is parked inside ServeHTTP until someone calls Done or
RequestTimeout expires, and on expiry it writes a 504 and drops everything the worker
writes afterwards, because the response writer only forwards while the context is
alive. So a handler that answers the request but never calls Done still produces a 504
for the client, and the correct answer it wrote is discarded.

The rule reports a callback that binds a meta.MessageWebRequest and never calls Done on
it, anywhere in the body: a deferred call counts, and so does a call in one branch,
because "which branch returns without it" is dataflow. Missing entirely is the form
worth reporting.

The second population is the mirror image. act.WebWorker already does this correctly:
its run loop intercepts meta.MessageWebRequest before the behavior sees it, calls the
verb method, and defers Done itself. A type embedding act.WebWorker that also handles
the raw message in HandleMessage is writing code the run loop makes unreachable, and in
HandleEvent it is unreachable for a second reason: the event branch delivers
gen.MessageEvent, never a raw request. That code is where the author thinks the request
is handled, so it looks handled while every real request goes to whichever verb method
they did not override, which answers 501.

HandleCall and HandleEvent are deliberately outside the Done population. The web
handler meta always delivers with Send, so a request that reaches either of those got
there because user code forwarded it, and the forwarder may well be the frame holding
the obligation. Only the three message callbacks are on the delivery path the meta
actually uses.

The fix for the first is to call Done, deferred at the top of the handling branch. For
the second it is to delete the dead case and implement HandleGet, HandlePost and the
rest.

Source: meta.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2008",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const (
	requestTypePath = "ergo.services/ergo/meta.MessageWebRequest"
	webWorkerPath   = "ergo.services/ergo/act.WebWorker"
)

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		bindings := requestBindings(pass, cb.Decl)
		if len(bindings) == 0 {
			continue
		}
		worker := embedsWebWorker(cb.Behavior)

		if worker {

			why, reportable := deadOnWorker(cb.Name)
			if reportable == false {
				continue
			}
			for _, b := range bindings {
				m.Report(pass, b.pos,
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb), Witness: "dead on a WebWorker",
					},
					"%s embeds act.WebWorker, so this meta.MessageWebRequest case is unreachable: %s; every real request goes to HandleGet, HandlePost and the other verb methods, which answer 501 until overridden",
					cb.Behavior.Obj().Name(), why)
			}
			continue
		}

		if doneRequired(cb.Name) == false {
			continue
		}

		for _, b := range bindings {
			if b.name == "" {

				m.Report(pass, b.pos,
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb), Witness: "request discarded",
					},
					"%s matches meta.MessageWebRequest without binding it, so Done can never be called: the HTTP goroutine stays parked inside ServeHTTP until RequestTimeout and the client gets a 504; bind the message and defer its Done",
					cb.Name)
				continue
			}
			if callsDone(pass, cb.Decl, b.name) {
				continue
			}
			m.Report(pass, b.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: "Done never called",
				},
				"%s handles meta.MessageWebRequest and never calls %s.Done(), so the HTTP goroutine stays parked inside ServeHTTP until RequestTimeout, the client gets a 504, and anything written to the response writer after the deadline is dropped; defer %s.Done() where the request is bound",
				cb.Name, b.name, b.name)
		}
	}
	return nil, nil
}

type binding struct {
	name string
	pos  token.Pos
}

func requestBindings(pass *analysis.Pass, decl *ast.FuncDecl) []binding {
	if decl.Body == nil {
		return nil
	}
	var out []binding

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.TypeSwitchStmt:
			bound := typeSwitchBinding(x)
			for _, stmt := range x.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if ok == false {
					continue
				}
				for _, expr := range clause.List {
					if isRequestType(pass, expr) == false {
						continue
					}

					name := bound
					if len(clause.List) > 1 {
						name = ""
					}
					out = append(out, binding{name: name, pos: expr.Pos()})
				}
			}
		case *ast.TypeAssertExpr:
			if x.Type == nil || isRequestType(pass, x.Type) == false {
				return true
			}
			out = append(out, binding{name: assertBinding(pass, decl, x), pos: x.Pos()})
		}
		return true
	})
	return out
}

func typeSwitchBinding(sw *ast.TypeSwitchStmt) string {
	as, ok := sw.Assign.(*ast.AssignStmt)
	if ok == false || len(as.Lhs) != 1 {
		return ""
	}
	id, isIdent := as.Lhs[0].(*ast.Ident)
	if isIdent == false || id.Name == "_" {
		return ""
	}
	return id.Name
}

func assertBinding(pass *analysis.Pass, decl *ast.FuncDecl, assert *ast.TypeAssertExpr) string {
	name := ""
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if name != "" {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Rhs) != 1 || as.Rhs[0] != ast.Expr(assert) {
			return true
		}
		id, isIdent := as.Lhs[0].(*ast.Ident)
		if isIdent && id.Name != "_" {
			name = id.Name
		}
		return false
	})
	return name
}

func callsDone(pass *analysis.Pass, decl *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok == false || sel.Sel.Name != "Done" {
			return true
		}

		if strings.HasPrefix(exprText(sel.X), name) {
			found = true
		}
		return true
	})
	if found {
		return true
	}

	return escapes(pass, decl, name)
}

func escapes(pass *analysis.Pass, decl *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		for _, arg := range call.Args {
			if id, isIdent := arg.(*ast.Ident); isIdent && id.Name == name {
				found = true
			}
		}
		return true
	})
	return found
}

func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	}
	return ""
}

func isRequestType(pass *analysis.Pass, e ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path()+"."+named.Obj().Name() == requestTypePath
}

func doneRequired(callback string) bool {
	switch callback {
	case "HandleMessage", "HandleMessageName", "HandleMessageAlias":
		return true
	}
	return false
}

func deadOnWorker(callback string) (string, bool) {
	switch callback {
	case "HandleMessage":
		return "the run loop matches the request before HandleMessage is called, handles it through the verb methods and defers Done itself", true
	case "HandleEvent":
		return "the event branch delivers gen.MessageEvent, so a raw request never reaches HandleEvent at all", true
	}
	return "", false
}

func embedsWebWorker(behavior *types.Named) bool {
	if behavior == nil {
		return false
	}
	st, ok := behavior.Underlying().(*types.Struct)
	if ok == false {
		return false
	}
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		if f.Anonymous() == false {
			continue
		}
		named, isNamed := types.Unalias(f.Type()).(*types.Named)
		if isNamed == false || named.Obj() == nil || named.Obj().Pkg() == nil {
			continue
		}
		if named.Obj().Pkg().Path()+"."+named.Obj().Name() == webWorkerPath {
			return true
		}
	}
	return false
}

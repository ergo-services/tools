package selfrequest

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1003"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1003",
	Doc: `A1003: a synchronous request addressed to the caller's own identity.

The word deadlock does not appear here, because it is not one, and the mechanism differs
by address form.

By PID the runtime refuses immediately: callPID returns ErrNotAllowed before any routing
and RouteCallPID has the same guard. That form belongs to the A2001 family, which reports
it as a call that cannot succeed.

By registered name, own ProcessID or own alias there is no self check anywhere. The
request is routed to this process's own mailbox, and the caller is already sitting in
WaitResponse, so nothing will ever pop it: run() is a compare-and-swap from Sleep that
fails while the caller waits. The call burns the full request timeout, five seconds by
default, and returns ErrTimeout. If the process has no registered name it fails fast with
ErrProcessUnknown instead, which is the better outcome of the two.

Only the intrinsic forms are reported, where the target expression is literally this
handle's own identity accessor: X.Call(X.Name()), a ProcessID composite built from
X.Name() with an own-node Node, or an alias from X.Aliases() or X.CreateAlias(). That
needs no notion of "the current actor" and no reachability, which is why it is sound.

The atom-constant form, matching a name the receiver passes to RegisterName, is
deliberately NOT reported. It is a heuristic: a conditional RegisterName means ownership
is dynamic and a follower calling that name is correct code, and a name that also appears
in a child spec or an application member spec usually belongs to a sibling. The corpus
count of the whole rule is zero either way, so it ships as a regression guard and keeps a
single defensible justification rather than trading it for findings that do not exist.

The Send family is never reported. A self-send is a heavily used idiom, including in the
framework's own tree, because it defers work to the next mailbox pass rather than waiting
for it.`,
	URL:      "https://docs.ergo.services/tools/argus#A1003",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var callMethods = map[string]bool{
	"Call":             true,
	"CallWithTimeout":  true,
	"CallWithPriority": true,
	"CallImportant":    true,
	"CallProcessID":    true,
	"CallAlias":        true,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok == false || fn.Body == nil {
				continue
			}
			singles := singleAssignments(fn.Body)

			var walk func(n ast.Node) bool
			walk = func(n ast.Node) bool {

				if _, isGo := n.(*ast.GoStmt); isGo {
					return false
				}
				call, isCall := n.(*ast.CallExpr)
				if isCall == false {
					return true
				}
				form, base, ok := selfTarget(pass, m, call, singles)
				if ok == false {
					return true
				}
				m.Report(pass, call.Pos(),
					ergomodel.Finding{
						Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: m.DeclID(fn) + ":" + form, Witness: form,
					},
					"this request is addressed to %s, which is this process's own %s, so it lands in the caller's own mailbox while the caller waits for the reply; nothing pops it and the call burns the full request timeout before returning ErrTimeout",
					base, form)
				return true
			}
			ast.Inspect(fn.Body, walk)
		}
	}
	return nil, nil
}

func selfTarget(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr,
	singles map[string]ast.Expr) (form string, base string, ok bool) {

	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false || len(call.Args) == 0 {
		return "", "", false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if isFunc == false || callMethods[fn.Name()] == false {
		return "", "", false
	}

	if isProcessMethod(fn) == false {
		return "", "", false
	}

	handle := handlePath(sel.X)
	if handle == "" {
		return "", "", false
	}
	target := call.Args[0]

	switch typeName(pass, target) {
	case "ergo.services/ergo/gen.Atom":
		if isAccessor(pass, target, handle, "Name") {
			return "registered name", handle + ".Name()", true
		}
	case "ergo.services/ergo/gen.ProcessID":
		if ok := selfProcessID(pass, target, handle); ok {
			return "ProcessID", "a ProcessID built from " + handle + ".Name()", true
		}
	case "ergo.services/ergo/gen.Alias":
		if idx, isIndex := target.(*ast.IndexExpr); isIndex {
			if isAccessor(pass, idx.X, handle, "Aliases") {
				return "alias", handle + ".Aliases()", true
			}
		}
		if id, isIdent := target.(*ast.Ident); isIdent {
			if rhs, found := singles[id.Name]; found && isAccessor(pass, rhs, handle, "CreateAlias") {
				return "alias", handle + ".CreateAlias()", true
			}
		}
	}
	return "", "", false
}

func selfProcessID(pass *analysis.Pass, target ast.Expr, handle string) bool {
	lit, isLit := target.(*ast.CompositeLit)
	if isLit == false {
		return false
	}
	name, node := false, false
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if isKV == false {
			continue
		}
		key, isIdent := kv.Key.(*ast.Ident)
		if isIdent == false {
			continue
		}
		switch key.Name {
		case "Name":
			name = isAccessor(pass, kv.Value, handle, "Name")
		case "Node":
			node = isOwnNode(pass, kv.Value, handle)
		}
	}
	return name && node
}

func isOwnNode(pass *analysis.Pass, e ast.Expr, handle string) bool {

	if outer, isCall := e.(*ast.CallExpr); isCall {
		if sel, isSel := outer.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Name" {
			if isAccessor(pass, sel.X, handle, "Node") {
				return true
			}
		}
	}

	if sel, isSel := e.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Node" {
		return isAccessor(pass, sel.X, handle, "PID")
	}
	return false
}

func isAccessor(pass *analysis.Pass, e ast.Expr, handle, method string) bool {
	call, isCall := e.(*ast.CallExpr)
	if isCall == false {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false || sel.Sel.Name != method {
		return false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if isFunc == false || isProcessMethod(fn) == false {
		return false
	}
	return handlePath(sel.X) == handle
}

func isProcessMethod(fn *types.Func) bool {
	return fn.Pkg() != nil && strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/")
}

func handlePath(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		inner := handlePath(x.X)
		if inner == "" {
			return ""
		}
		return inner + "." + x.Sel.Name
	}
	return ""
}

func typeName(pass *analysis.Pass, e ast.Expr) string {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return ""
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

func singleAssignments(body *ast.BlockStmt) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	count := map[string]int{}
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Rhs) != 1 {
			return true
		}
		for _, lhs := range as.Lhs {
			id, isIdent := lhs.(*ast.Ident)
			if isIdent == false {
				continue
			}
			count[id.Name]++
			out[id.Name] = as.Rhs[0]
		}
		return true
	})
	for name, n := range count {
		if n > 1 {
			delete(out, name)
		}
	}
	return out
}

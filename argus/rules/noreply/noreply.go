package noreply

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2002"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2002",
	Doc: `A2002: a HandleCall that can never reply.

Returning (nil, nil) is legitimate: the runtime sends nothing for a nil result, which is
how a deferred reply works. The callback stores the caller's PID and ref, and something
later calls SendResponse. So the shape alone is not a defect.

It becomes one when no reply is possible at all. The rule requires three things
together: every return path is (nil, nil), the behavior type declares no method that
replies, and the gen.Ref parameter is never used. That last one is the soundness gate
rather than a heuristic: waitResponse drops any response whose ref differs from the one
it is waiting for, so a callback that never even looks at its ref cannot participate in
a reply, and no later code can reply on its behalf. The caller then waits out its full
request timeout and gets ErrTimeout.

A use of ref inside a log argument still counts as unused, because logging a token is
not keeping it.

The reply check is deliberately conservative. It reads the explicitly declared methods
of the behavior type, not the promoted method set: the promoted set contains
gen.Process.SendResponse itself, which would mark every actor as replying and silence
the rule completely. The cost is recall on a split-handle type, where HandleCall goes
quiet because HandleCallName replies, and that is the right direction to be wrong in.

Two populations are excluded outright. Behavior types declared inside the framework are
the documented unhandled-request fallbacks that exist to be overridden. Test files are
excluded unconditionally, regardless of the tests setting, because a no-reply stub is a
normal thing for a test to contain.

The fix is to decline explicitly: SendResponseError(from, ref, gen.ErrUnsupported), or
return the error as the result. Returning it as the reason instead terminates the
process, which is A1008.`,
	URL:      "https://docs.ergo.services/tools/argus#A2002",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const ergoPrefix = "ergo.services/ergo/"

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBHandleCall {
			continue
		}

		if replyShaped(pass, cb.Decl) == false {
			continue
		}

		if fromFramework(cb.Behavior) {
			continue
		}

		if strings.HasSuffix(pass.Fset.Position(cb.Decl.Pos()).Filename, "_test.go") {
			continue
		}
		if namedResultAssigned(pass, cb.Decl) {
			continue
		}
		if allPathsTwoNils(cb.Decl) == false {
			continue
		}
		ref := paramOfType(pass, cb.Decl, "ergo.services/ergo/gen.Ref")
		if ref != nil && usedOutsideLog(pass, cb.Decl.Body, ref) {
			continue
		}
		if behaviorReplies(m, cb.Behavior) {
			continue
		}

		witness := "discards request"
		if request := paramNamed(pass, cb.Decl, "request", "message"); request != nil &&
			usedOutsideLog(pass, cb.Decl.Body, request) {
			witness = "inspects request"
		}

		m.Report(pass, cb.Decl.Name.Pos(),
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(cb), Witness: witness,
			},
			"%s returns (nil, nil) on every path, never uses ref, and no method of %s replies, so no response is ever sent and every caller waits out its full request timeout before getting ErrTimeout; decline with SendResponseError(from, ref, gen.ErrUnsupported), or return the error as the result",
			cb.Name, cb.Behavior.Obj().Name())
	}
	return nil, nil
}

func replyShaped(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	obj, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if ok == false {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	if ok == false || sig.Results().Len() != 2 {
		return false
	}
	first, ok := types.Unalias(sig.Results().At(0).Type()).(*types.Interface)
	if ok == false || first.NumMethods() != 0 {
		return false
	}
	return types.Unalias(sig.Results().At(1).Type()).String() == "error"
}

func fromFramework(behavior *types.Named) bool {
	if behavior == nil || behavior.Obj() == nil || behavior.Obj().Pkg() == nil {
		return false
	}
	return strings.HasPrefix(behavior.Obj().Pkg().Path(), ergoPrefix)
}

func allPathsTwoNils(decl *ast.FuncDecl) bool {
	returns := 0
	ok := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		ret, isRet := n.(*ast.ReturnStmt)
		if isRet == false {
			return true
		}
		returns++
		switch len(ret.Results) {
		case 0:

		case 2:
			if isNil(ret.Results[0]) == false || isNil(ret.Results[1]) == false {
				ok = false
			}
		default:
			ok = false
		}
		return true
	})
	return ok && returns > 0
}

func namedResultAssigned(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	if decl.Type.Results == nil {
		return false
	}
	name := ""
	for _, field := range decl.Type.Results.List {
		if len(field.Names) > 0 {
			name = field.Names[0].Name
			break
		}
	}
	if name == "" || name == "_" {
		return false
	}
	assigned := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, isIdent := lhs.(*ast.Ident); isIdent && id.Name == name {
				assigned = true
			}
		}
		return true
	})
	return assigned
}

func paramOfType(pass *analysis.Pass, decl *ast.FuncDecl, path string) types.Object {
	if decl.Type.Params == nil {
		return nil
	}
	for _, field := range decl.Type.Params.List {
		t := pass.TypesInfo.TypeOf(field.Type)
		if t == nil {
			continue
		}
		named, ok := types.Unalias(t).(*types.Named)
		if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
			continue
		}
		if named.Obj().Pkg().Path()+"."+named.Obj().Name() != path {
			continue
		}
		for _, id := range field.Names {
			if id.Name == "_" {
				continue
			}
			if obj := pass.TypesInfo.Defs[id]; obj != nil {
				return obj
			}
		}
	}
	return nil
}

func paramNamed(pass *analysis.Pass, decl *ast.FuncDecl, names ...string) types.Object {
	if decl.Type.Params == nil {
		return nil
	}
	for _, want := range names {
		for _, field := range decl.Type.Params.List {
			for _, id := range field.Names {
				if id.Name != want {
					continue
				}
				if obj := pass.TypesInfo.Defs[id]; obj != nil {
					return obj
				}
			}
		}
	}
	return nil
}

func usedOutsideLog(pass *analysis.Pass, body *ast.BlockStmt, obj types.Object) bool {
	logRanges := logArgumentRanges(pass, body)
	used := false
	ast.Inspect(body, func(n ast.Node) bool {
		if used {
			return false
		}
		id, ok := n.(*ast.Ident)
		if ok == false || pass.TypesInfo.Uses[id] != obj {
			return true
		}
		for _, r := range logRanges {
			if id.Pos() >= r[0] && id.End() <= r[1] {
				return true
			}
		}
		used = true
		return false
	})
	return used
}

func logArgumentRanges(pass *analysis.Pass, body *ast.BlockStmt) [][2]token.Pos {
	var out [][2]token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok == false || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if ok == false {
			return true
		}
		fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		if ok == false || fn.Pkg() == nil || fn.Pkg().Path() != "ergo.services/ergo/gen" {
			return true
		}
		if recvName(fn) != "Log" {
			return true
		}
		out = append(out, [2]token.Pos{call.Args[0].Pos(), call.Args[len(call.Args)-1].End()})
		return true
	})
	return out
}

func behaviorReplies(m *ergomodel.Model, behavior *types.Named) bool {
	if behavior == nil {
		return false
	}
	for i := 0; i < behavior.NumMethods(); i++ {
		if m.Behavior(behavior.Method(i)).Replies {
			return true
		}
	}
	return false
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func recvName(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return ""
	}
	t := sig.Recv().Type()
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false {
		return ""
	}
	return named.Obj().Name()
}

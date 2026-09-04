package doublereply

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2003"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2003",
	Doc: `A2003: HandleCall replies twice for one request.

The explicit reply and the returned result are alternatives, not complements. When the
callback returns a non-nil result the runtime replies with it itself, addressed to the
same (from, ref) the callback was handed. So an explicit SendResponse followed by a
non-nil return sends two responses for one request.

The cost is not redundancy. The caller's response channel is buffered at ten, and
waitResponse silently drops any response whose ref it is no longer waiting for, logging
it only in a verbose build. Ten stale replies fill that buffer, and then the next
legitimate reply to that caller is refused with ErrResponseIgnored and its Call times
out instead. Worse for an important reply: the auto-ack is sent only on the
matching-ref path, so a duplicate important response is never acked and its sender
blocks for the full request timeout, five seconds of the replying actor's mailbox per
occurrence.

Narrow by construction. The reply has to be addressed to this callback's own from and
ref parameters, resolved by parameter type rather than position because
HandleCallName and HandleCallAlias shift them: a reply to a stored or foreign ref is the
legitimate flush pattern, where a handler answers an earlier pending request and returns
this one's result.

The returned result must be provably non-nil from its own type or literal form. An
interface or pointer typed variable is never reported, because deciding whether it is nil
is dataflow.

The reason slot must be nil or TerminateReasonNormal, and those are exactly the two
cases where the runtime replies. Any other non-nil reason makes it suppress the reply, so
there is no second one. That leaves a documented blind spot: a Supervisor replies whenever
the result is non-nil regardless of reason, and this rule stays silent there.

Precedence is structural: the reply must be a direct statement of a block, case body or
select arm enclosing the return, or a defer registered before it. The rule never looks
inside a preceding if, for, switch or select, which is what keeps mutually exclusive
branches silent: replying in one case and returning a result in another is correct code.

No fix is offered. The repair is a choice between deleting the returned value and
deleting the explicit reply, and the tool cannot make it.`,
	URL:      "https://docs.ergo.services/tools/argus#A2003",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const ergoPrefix = "ergo.services/ergo/"

var replyMethods = map[string]bool{
	"SendResponse":               true,
	"SendResponseImportant":      true,
	"SendResponseError":          true,
	"SendResponseErrorImportant": true,
}

type reply struct {
	method string
	pos    token.Pos
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBHandleCall || resultCount(cb.Decl) != 2 {
			continue
		}
		from := paramOfType(pass, cb.Decl, "ergo.services/ergo/gen.PID")
		ref := paramOfType(pass, cb.Decl, "ergo.services/ergo/gen.Ref")
		if from == nil || ref == nil {
			continue
		}
		aliases := singleAssignedParams(pass, cb.Decl.Body, from, ref)

		s := &scanner{pass: pass, m: m, cb: cb, from: from, ref: ref, aliases: aliases}
		s.scan(cb.Decl.Body.List, nil)
	}
	return nil, nil
}

type scanner struct {
	pass    *analysis.Pass
	m       *ergomodel.Model
	cb      *ergomodel.Callback
	from    types.Object
	ref     types.Object
	aliases map[types.Object]types.Object
}

func (s *scanner) scan(list []ast.Stmt, seen []reply) {
	for _, stmt := range list {
		switch x := stmt.(type) {
		case *ast.ReturnStmt:
			s.check(x, seen)

		case *ast.ExprStmt:
			if r, ok := s.replyCall(x.X); ok {
				seen = append(seen, r)
			}

		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				if r, ok := s.replyCall(x.Rhs[0]); ok {
					seen = append(seen, r)
				}
			}

		case *ast.DeferStmt:

			if r, ok := s.replyCall(x.Call); ok {
				seen = append(seen, r)
			}

		case *ast.IfStmt:
			if r, ok := s.initReply(x.Init); ok {
				seen = append(seen, r)
			}
			s.scan(x.Body.List, clone(seen))
			s.scanElse(x.Else, clone(seen))

		case *ast.SwitchStmt:
			if r, ok := s.initReply(x.Init); ok {
				seen = append(seen, r)
			}
			s.scanClauses(x.Body, clone(seen))

		case *ast.TypeSwitchStmt:
			s.scanClauses(x.Body, clone(seen))

		case *ast.SelectStmt:
			s.scanClauses(x.Body, clone(seen))

		case *ast.ForStmt:
			s.scan(x.Body.List, clone(seen))

		case *ast.RangeStmt:
			s.scan(x.Body.List, clone(seen))

		case *ast.BlockStmt:
			s.scan(x.List, clone(seen))
		}
	}
}

func (s *scanner) scanElse(stmt ast.Stmt, seen []reply) {
	switch x := stmt.(type) {
	case *ast.BlockStmt:
		s.scan(x.List, seen)
	case *ast.IfStmt:
		s.scan([]ast.Stmt{x}, seen)
	}
}

func (s *scanner) scanClauses(body *ast.BlockStmt, seen []reply) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		switch c := stmt.(type) {
		case *ast.CaseClause:
			s.scan(c.Body, clone(seen))
		case *ast.CommClause:
			s.scan(c.Body, clone(seen))
		}
	}
}

func (s *scanner) initReply(init ast.Stmt) (reply, bool) {
	switch x := init.(type) {
	case *ast.ExprStmt:
		return s.replyCall(x.X)
	case *ast.AssignStmt:
		if len(x.Rhs) == 1 {
			return s.replyCall(x.Rhs[0])
		}
	}
	return reply{}, false
}

func (s *scanner) check(ret *ast.ReturnStmt, seen []reply) {
	if len(seen) == 0 || len(ret.Results) != 2 {
		return
	}

	if isNil(ret.Results[1]) == false && isTerminateNormal(s.pass, ret.Results[1]) == false {
		return
	}
	if definitelyNonNil(s.pass, ret.Results[0]) == false {
		return
	}
	first := seen[0]
	s.m.Report(s.pass, ret.Results[0].Pos(),
		ergomodel.Finding{
			Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
			ID: ergomodel.CallbackID(s.cb) + ":" + first.method, Witness: first.method,
		},
		"%s already replied to this request with %s at line %d, and returning a non-nil result makes the runtime send a second response for the same ref; the caller drops the duplicate but it costs a slot in a channel of ten, after which a legitimate reply to that caller is refused and its Call times out",
		s.cb.Name, first.method, s.pass.Fset.Position(first.pos).Line)
}

func (s *scanner) replyCall(e ast.Expr) (reply, bool) {
	call, ok := e.(*ast.CallExpr)
	if ok == false || len(call.Args) < 2 {
		return reply{}, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false || replyMethods[sel.Sel.Name] == false {
		return reply{}, false
	}
	fn, ok := s.pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false || fn.Pkg() == nil {
		return reply{}, false
	}
	if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return reply{}, false
	}
	if s.isParam(call.Args[0], s.from) == false || s.isParam(call.Args[1], s.ref) == false {
		return reply{}, false
	}
	return reply{method: sel.Sel.Name, pos: call.Pos()}, true
}

func (s *scanner) isParam(e ast.Expr, want types.Object) bool {
	id, ok := e.(*ast.Ident)
	if ok == false {
		return false
	}
	obj := s.pass.TypesInfo.Uses[id]
	if obj == want {
		return true
	}
	return obj != nil && s.aliases[obj] == want
}

func singleAssignedParams(pass *analysis.Pass, body *ast.BlockStmt, want ...types.Object) map[types.Object]types.Object {
	wanted := map[types.Object]bool{}
	for _, w := range want {
		wanted[w] = true
	}
	out := map[types.Object]types.Object{}
	count := map[types.Object]int{}

	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if ok == false {
			return true
		}
		obj := pass.TypesInfo.Defs[lhs]
		if obj == nil {
			obj = pass.TypesInfo.Uses[lhs]
		}
		if obj == nil {
			return true
		}
		count[obj]++
		if rhs, isIdent := as.Rhs[0].(*ast.Ident); isIdent {
			if src := pass.TypesInfo.Uses[rhs]; src != nil && wanted[src] {
				out[obj] = src
			}
		}
		return true
	})
	for obj, n := range count {
		if n > 1 {
			delete(out, obj)
		}
	}
	return out
}

func definitelyNonNil(pass *analysis.Pass, e ast.Expr) bool {

	if isNil(e) {
		return false
	}

	switch x := e.(type) {
	case *ast.CompositeLit:
		return true
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		if x.Op == token.AND {
			return true
		}
	}
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return false
	}
	u := types.Unalias(t)
	if _, ok := u.(*types.TypeParam); ok {
		return false
	}
	switch u.Underlying().(type) {
	case *types.Interface, *types.Pointer, *types.Slice, *types.Map,
		*types.Chan, *types.Signature:
		return false
	}
	if b, ok := u.Underlying().(*types.Basic); ok {
		switch b.Kind() {
		case types.UnsafePointer, types.UntypedNil, types.Invalid:
			return false
		}
	}
	return true
}

func resultCount(decl *ast.FuncDecl) int {
	if decl.Type.Results == nil {
		return 0
	}
	n := 0
	for _, f := range decl.Type.Results.List {
		if len(f.Names) == 0 {
			n++
			continue
		}
		n += len(f.Names)
	}
	return n
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

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

func isTerminateNormal(pass *analysis.Pass, e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false || sel.Sel.Name != "TerminateReasonNormal" {
		return false
	}
	obj := pass.TypesInfo.Uses[sel.Sel]
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "ergo.services/ergo/gen"
}

func clone(in []reply) []reply {
	out := make([]reply, len(in))
	copy(out, in)
	return out
}

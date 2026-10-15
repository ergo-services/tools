package terminatehalfbuilt

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2014a"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2014a",
	Doc: `A2014a: Terminate dereferencing something Init may never have assigned.

Terminate does not only run after a working process stops. When Init returns an error
the spawn path calls cleanupProcess and then ProcessTerminate with that same error,
on a fresh goroutine, against the receiver Init abandoned. When Init exceeds its
timeout the other path does the same with TerminateReasonKill. So every field assigned
after the first thing in Init that can fail is nil in a Terminate that runs on those
paths, and a Terminate written for the healthy case dereferences it.

What that costs is worse than it looks. The failure path runs Terminate on its own
goroutine, and the panic boundary there is the same build-tag switch as everywhere
else: the recover is installed only when lib.Recover is on. Under -tags=norecover a
nil dereference in a Terminate reached this way is a panic on a bare goroutine, which
takes the node down. With recovery on it is a Panic log, and the original Init error
that caused all of this is what the spawner sees, so the log and the returned error
disagree about what went wrong.

A meta process is excluded, because the same claim is false there. A meta's Init is
called synchronously from SpawnMeta, and when it returns an error SpawnMeta returns
immediately: the "go m.start()" that would eventually invoke Terminate is never
reached, so a meta whose Init failed has no Terminate at all. The framework's own
meta/port.go is the shape this exclusion protects, and it reads like the defect: it
guards on cmd, which is assigned before anything can fail, while the three pipes it
closes are the fields that can be nil. For a meta that guard is redundant rather than
misplaced.

The predicate is deliberately narrow. A field is late if every assignment to it in
Init sits after the first statement in Init that can return early, since a field
assigned before that point is set on every path that reaches Terminate at all. A late
field is reported when Terminate dereferences it and the type can be nil: a call
through it, a selection through it, an index, a receive, or an explicit star. Passing
it to something is not a dereference, because a nil pointer argument is only a problem
in the callee's frame, and the callee is judged on its own.

Any nil comparison against the field anywhere in Terminate counts as a guard, without
asking whether the guard dominates the use. Getting that wrong in the quiet direction
costs recall on code that is already thinking about the problem, and getting it wrong
in the loud direction reports code that already has the fix.

A guard also covers every field Init assigns before the guarded one, and that is not
leniency: Init assigns in order, so if the later field is non nil the earlier one was
reached. Real code guards the last thing it built and then uses everything, which is
correct and would otherwise be reported. The direction matters, which is why the
implication runs only from a late guard: a guard on a field assigned before anything
can fail says nothing at all, since it is always true, and that is the shape a
misplaced guard has.

A dereference that only runs once a condition on another field has held is guarded
too, when that field cannot be set on a half built receiver. Init and Terminate are
the only callbacks that run on it, so a field written by nothing either of them can
reach still holds its zero value there, and a condition that is false at the zero
value (a bool field, != nil, != 0, len > 0, and their negations) keeps the dereference
behind it off the failure path. The field must be unexported and written only by
methods of the behavior, never by its factory or a helper; the receiver must not
escape from Init or Terminate; and a method that is passed around as a value or could
be reached through an interface counts as reachable. The condition counts when it
encloses the dereference, or when an earlier statement leaves on its negation.

One field is reported once per behavior, at its first unguarded dereference.

An application is excluded for a different reason: its Terminate is never called after
its Init fails or times out. The node runs Init, and on an error it gives the state
back without calling Terminate; finalize calls Terminate only for an incarnation whose
Init succeeded.

The fix is the guard, or assigning the field before anything in Init can fail.

Source: process.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2014a",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	inits := map[*types.Named]*ast.FuncDecl{}
	terminates := map[*types.Named]*ergomodel.Callback{}
	for _, cb := range m.Callbacks {

		if cb.Meta || cb.Application {
			continue
		}
		switch cb.Name {
		case "Init":
			inits[cb.Behavior] = cb.Decl
		case "Terminate":
			terminates[cb.Behavior] = cb
		}
	}

	var writes *packageWrites
	for behavior, cb := range terminates {
		init := inits[behavior]
		if init == nil {
			continue
		}
		late := lateFields(pass, init)
		if len(late) == 0 {
			continue
		}
		guarded := guardedFields(pass, cb.Decl, late)

		var zero *zeroFields
		reported := map[types.Object]bool{}
		for _, use := range dereferences(pass, cb.Decl) {
			if _, isLate := late[use.field]; isLate == false || guarded[use.field] || reported[use.field] {
				continue
			}
			if zero == nil {
				if writes == nil {
					writes = scanWrites(pass, m)
				}
				zero = writes.zero(behavior, cb.Decl, init, cb.Decl)
			}
			if zero.dominates(cb.Decl, use.pos) {
				continue
			}
			reported[use.field] = true
			m.Report(pass, use.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: use.field.Name(),
				},
				"Terminate dereferences %s, which Init assigns only after its first fallible step, and Terminate also runs when Init fails or times out, against exactly that half built receiver; guard the field, or assign it before anything in Init can return an error",
				use.field.Name())
		}
	}
	return nil, nil
}

func lateFields(pass *analysis.Pass, init *ast.FuncDecl) map[types.Object]token.Pos {
	if init.Body == nil {
		return nil
	}
	cut, found := firstFallible(init.Body)
	if found == false {

		return nil
	}

	early := map[types.Object]bool{}
	late := map[types.Object]token.Pos{}
	ast.Inspect(init.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if ok == false {
			return true
		}
		for _, lhs := range as.Lhs {
			sel, isSel := lhs.(*ast.SelectorExpr)
			if isSel == false {
				continue
			}
			obj := pass.TypesInfo.Uses[sel.Sel]
			if obj == nil {
				continue
			}
			if _, isField := obj.(*types.Var); isField == false {
				continue
			}
			if as.Pos() < cut {
				early[obj] = true
				continue
			}
			if prev, seen := late[obj]; seen == false || as.Pos() < prev {
				late[obj] = as.Pos()
			}
		}
		return true
	})

	for obj := range early {
		delete(late, obj)
	}
	return late
}

func firstFallible(body *ast.BlockStmt) (token.Pos, bool) {
	for i, stmt := range body.List {
		last := i == len(body.List)-1
		if returnsEarly(stmt) == false {
			continue
		}
		if last {

			if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
				continue
			}
		}
		return stmt.Pos(), true
	}
	return 0, false
}

func returnsEarly(stmt ast.Stmt) bool {
	found := false
	ast.Inspect(stmt, func(n ast.Node) bool {
		if found {
			return false
		}
		if _, isLit := n.(*ast.FuncLit); isLit {
			return false
		}
		if _, isReturn := n.(*ast.ReturnStmt); isReturn {
			found = true
		}
		return true
	})
	return found
}

type use struct {
	field types.Object
	pos   token.Pos
}

func dereferences(pass *analysis.Pass, decl *ast.FuncDecl) []use {
	if decl.Body == nil {
		return nil
	}
	var out []use
	note := func(e ast.Expr) {
		sel, ok := e.(*ast.SelectorExpr)
		if ok == false {
			return
		}
		obj := pass.TypesInfo.Uses[sel.Sel]
		if obj == nil || nilable(obj.Type()) == false {
			return
		}
		out = append(out, use{field: obj, pos: sel.Pos()})
	}

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:

			note(x.X)
		case *ast.IndexExpr:
			note(x.X)
		case *ast.StarExpr:
			note(x.X)
		case *ast.UnaryExpr:
			if x.Op == token.ARROW {
				note(x.X)
			}
		case *ast.SendStmt:
			note(x.Chan)
		}
		return true
	})
	return out
}

func nilable(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Signature, *types.Chan:
		return true
	case *types.Map:

		return true
	case *types.Slice:

		return true
	default:
		_ = u
		return false
	}
}

func guardedFields(pass *analysis.Pass, decl *ast.FuncDecl,
	late map[types.Object]token.Pos) map[types.Object]bool {

	out := nilChecked(pass, decl)
	for guard := range out {
		at, isLate := late[guard]
		if isLate == false {
			continue
		}
		for field, pos := range late {
			if pos <= at {
				out[field] = true
			}
		}
	}
	return out
}

func nilChecked(pass *analysis.Pass, decl *ast.FuncDecl) map[types.Object]bool {
	out := map[types.Object]bool{}
	if decl.Body == nil {
		return out
	}
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if ok == false || (bin.Op != token.EQL && bin.Op != token.NEQ) {
			return true
		}
		for _, side := range []ast.Expr{bin.X, bin.Y} {
			sel, isSel := side.(*ast.SelectorExpr)
			if isSel == false {
				continue
			}
			if isNil(bin.X) == false && isNil(bin.Y) == false {
				continue
			}
			if obj := pass.TypesInfo.Uses[sel.Sel]; obj != nil {
				out[obj] = true
			}
		}
		return true
	})
	return out
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

type packageWrites struct {
	pass    *analysis.Pass
	m       *ergomodel.Model
	writers map[*types.Var][]*ast.FuncDecl
	values  []*ast.FuncDecl
	methods map[string][]*ast.FuncDecl
}

func scanWrites(pass *analysis.Pass, m *ergomodel.Model) *packageWrites {
	w := &packageWrites{
		pass:    pass,
		m:       m,
		writers: map[*types.Var][]*ast.FuncDecl{},
		methods: map[string][]*ast.FuncDecl{},
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil {
				w.methods[fd.Name.Name] = append(w.methods[fd.Name.Name], fd)
			}
		}
	}

	valued := map[*ast.FuncDecl]bool{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {

			fd, _ := decl.(*ast.FuncDecl)
			called := map[*ast.Ident]bool{}
			ast.Inspect(decl, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if id := calleeIdent(x.Fun); id != nil {
						called[id] = true
					}
				case *ast.Ident:
					fn, ok := pass.TypesInfo.Uses[x].(*types.Func)
					if ok == false || called[x] {
						break
					}
					for _, target := range w.targets(fn) {
						if valued[target] == false {
							valued[target] = true
							w.values = append(w.values, target)
						}
					}
				case *ast.AssignStmt:
					for _, lhs := range x.Lhs {
						w.write(fd, lhs)
					}
				case *ast.IncDecStmt:
					w.write(fd, x.X)
				case *ast.RangeStmt:
					if x.Tok == token.ASSIGN {
						w.write(fd, x.Key)
						w.write(fd, x.Value)
					}
				case *ast.UnaryExpr:
					if x.Op == token.AND {
						w.write(fd, x.X)
					}
				case *ast.SliceExpr:
					if _, isArray := underlying(pass.TypesInfo.TypeOf(x.X)).(*types.Array); isArray {
						w.write(fd, x.X)
					}
				case *ast.CompositeLit:
					w.literal(fd, x)
				case *ast.SelectorExpr:
					sel, ok := pass.TypesInfo.Selections[x]
					if ok == false || sel.Kind() != types.MethodVal {
						break
					}
					sig, ok := sel.Obj().Type().(*types.Signature)
					if ok == false || sig.Recv() == nil {
						break
					}
					_, ptrRecv := sig.Recv().Type().(*types.Pointer)
					_, ptrBase := underlying(pass.TypesInfo.TypeOf(x.X)).(*types.Pointer)
					if ptrRecv && ptrBase == false {
						w.write(fd, x.X)
					}
				}
				return true
			})
		}
	}
	return w
}

func calleeIdent(fun ast.Expr) *ast.Ident {
	switch x := ast.Unparen(fun).(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return x.Sel
	case *ast.IndexExpr:
		return calleeIdent(x.X)
	case *ast.IndexListExpr:
		return calleeIdent(x.X)
	}
	return nil
}

func underlying(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	return t.Underlying()
}

func (w *packageWrites) targets(fn *types.Func) []*ast.FuncDecl {
	if decl := w.m.DeclOf(fn.Origin()); decl != nil {
		return []*ast.FuncDecl{decl}
	}
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil || types.IsInterface(sig.Recv().Type()) == false {
		return nil
	}
	return w.methods[fn.Name()]
}

func (w *packageWrites) write(fd *ast.FuncDecl, e ast.Expr) {
	switch x := ast.Unparen(e).(type) {
	case *ast.SelectorExpr:
		if field, ok := w.pass.TypesInfo.Uses[x.Sel].(*types.Var); ok && field.IsField() {
			w.note(fd, field)
		}
	case *ast.IndexExpr:
		w.write(fd, x.X)
	case *ast.StarExpr:
		ptr, ok := underlying(w.pass.TypesInfo.TypeOf(x.X)).(*types.Pointer)
		if ok == false {
			break
		}
		if st, ok := ptr.Elem().Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				w.note(fd, st.Field(i))
			}
		}
	}
}

func (w *packageWrites) literal(fd *ast.FuncDecl, lit *ast.CompositeLit) {
	t := underlying(w.pass.TypesInfo.TypeOf(lit))
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem().Underlying()
	}
	st, ok := t.(*types.Struct)
	if ok == false {
		return
	}
	for i, elt := range lit.Elts {
		kv, keyed := elt.(*ast.KeyValueExpr)
		if keyed == false {
			if i < st.NumFields() {
				w.note(fd, st.Field(i))
			}
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok {
			if field, ok := w.pass.TypesInfo.Uses[id].(*types.Var); ok && field.IsField() {
				w.note(fd, field)
			}
		}
	}
}

func (w *packageWrites) note(fd *ast.FuncDecl, field *types.Var) {
	w.writers[field] = append(w.writers[field], fd)
	if st, ok := field.Type().Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			w.note(fd, st.Field(i))
		}
	}
}

type zeroFields struct {
	w        *packageWrites
	behavior *types.Named
	recv     types.Object
	reach    map[*ast.FuncDecl]bool
	escaped  bool
}

func (w *packageWrites) zero(behavior *types.Named, terminate *ast.FuncDecl, roots ...*ast.FuncDecl) *zeroFields {
	z := &zeroFields{w: w, behavior: behavior, recv: receiverObject(w.pass, terminate), reach: map[*ast.FuncDecl]bool{}}
	queue := append(append([]*ast.FuncDecl{}, roots...), w.values...)
	for len(queue) > 0 {
		fd := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if z.reach[fd] {
			continue
		}
		z.reach[fd] = true
		if z.leaks(fd) {
			z.escaped = true
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				if fn, ok := w.pass.TypesInfo.Uses[id].(*types.Func); ok {
					queue = append(queue, w.targets(fn)...)
				}
			}
			return true
		})
	}
	return z
}

func receiverObject(pass *analysis.Pass, decl *ast.FuncDecl) types.Object {
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return nil
	}
	return pass.TypesInfo.Defs[decl.Recv.List[0].Names[0]]
}

func (z *zeroFields) ownMethod(fd *ast.FuncDecl) bool {
	if fd == nil || fd.Recv == nil {
		return false
	}
	fn, ok := z.w.pass.TypesInfo.Defs[fd.Name].(*types.Func)
	if ok == false {
		return false
	}
	t := types.Unalias(fn.Type().(*types.Signature).Recv().Type())
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	named, ok := t.(*types.Named)
	return ok && named.Origin() == z.behavior.Origin()
}

func (z *zeroFields) leaks(fd *ast.FuncDecl) bool {
	if z.ownMethod(fd) == false {
		return false
	}
	recv := receiverObject(z.w.pass, fd)
	if recv == nil || fd.Body == nil {
		return false
	}
	base := map[*ast.Ident]bool{}
	leaked := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := ast.Unparen(x.X).(*ast.Ident); ok {
				base[id] = true
			}
		case *ast.Ident:
			if base[x] == false && z.w.pass.TypesInfo.Uses[x] == recv {
				leaked = true
			}
		}
		return leaked == false
	})
	return leaked
}

func (z *zeroFields) cold(field *types.Var) bool {
	if z.escaped || field.Exported() || field.Pkg() != z.w.pass.Pkg {
		return false
	}
	for _, fd := range z.w.writers[field] {
		if fd == nil || z.reach[fd] || z.ownMethod(fd) == false {
			return false
		}
	}
	return true
}

func (z *zeroFields) field(e ast.Expr) (*types.Var, bool) {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if ok == false || z.recv == nil {
		return nil, false
	}
	id, ok := ast.Unparen(sel.X).(*ast.Ident)
	if ok == false || z.w.pass.TypesInfo.Uses[id] != z.recv {
		return nil, false
	}
	field, ok := z.w.pass.TypesInfo.Uses[sel.Sel].(*types.Var)
	if ok == false || field.IsField() == false || z.cold(field) == false {
		return nil, false
	}
	return field, true
}

func (z *zeroFields) zeroOf(e ast.Expr) (constant.Value, bool) {
	e = ast.Unparen(e)
	if call, ok := e.(*ast.CallExpr); ok {
		id, ok := ast.Unparen(call.Fun).(*ast.Ident)
		if ok == false || len(call.Args) != 1 {
			return nil, false
		}
		if b, ok := z.w.pass.TypesInfo.Uses[id].(*types.Builtin); ok == false || (b.Name() != "len" && b.Name() != "cap") {
			return nil, false
		}
		field, ok := z.field(call.Args[0])
		if ok == false {
			return nil, false
		}
		switch t := field.Type().Underlying().(type) {
		case *types.Slice, *types.Map, *types.Chan:
			return constant.MakeInt64(0), true
		case *types.Basic:
			if t.Info()&types.IsString != 0 {
				return constant.MakeInt64(0), true
			}
		}
		return nil, false
	}
	field, ok := z.field(e)
	if ok == false {
		return nil, false
	}
	switch t := field.Type().Underlying().(type) {
	case *types.Basic:
		switch {
		case t.Info()&types.IsBoolean != 0:
			return constant.MakeBool(false), true
		case t.Info()&types.IsString != 0:
			return constant.MakeString(""), true
		case t.Info()&types.IsInteger != 0:
			return constant.MakeInt64(0), true
		case t.Info()&types.IsFloat != 0:
			return constant.MakeFloat64(0), true
		}
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan, *types.Signature:
		return constant.MakeUnknown(), true
	}
	return nil, false
}

func (z *zeroFields) implies(e ast.Expr, want bool) bool {
	switch x := ast.Unparen(e).(type) {
	case *ast.UnaryExpr:
		if x.Op == token.NOT {
			return z.implies(x.X, want == false)
		}
	case *ast.BinaryExpr:
		switch x.Op {
		case token.LAND:
			if want {
				return z.implies(x.X, true) || z.implies(x.Y, true)
			}
			return z.implies(x.X, false) && z.implies(x.Y, false)
		case token.LOR:
			if want {
				return z.implies(x.X, true) && z.implies(x.Y, true)
			}
			return z.implies(x.X, false) || z.implies(x.Y, false)
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			if zero, ok := z.zeroOf(x.X); ok {
				if holds, ok := z.holds(zero, x.Op, x.Y, false); ok {
					return holds != want
				}
			}
			if zero, ok := z.zeroOf(x.Y); ok {
				if holds, ok := z.holds(zero, x.Op, x.X, true); ok {
					return holds != want
				}
			}
		}
	case *ast.SelectorExpr:
		if zero, ok := z.zeroOf(x); ok && zero.Kind() == constant.Bool {
			return constant.BoolVal(zero) != want
		}
	}
	return false
}

func (z *zeroFields) holds(zero constant.Value, op token.Token, other ast.Expr, swapped bool) (bool, bool) {
	tv, ok := z.w.pass.TypesInfo.Types[other]
	if ok == false {
		return false, false
	}
	if zero.Kind() == constant.Unknown {
		if tv.IsNil() == false || (op != token.EQL && op != token.NEQ) {
			return false, false
		}
		return op == token.EQL, true
	}
	if tv.Value == nil || tv.Value.Kind() == constant.Unknown {
		return false, false
	}
	if swapped {
		return constant.Compare(tv.Value, op, zero), true
	}
	return constant.Compare(zero, op, tv.Value), true
}

func (z *zeroFields) dominates(decl *ast.FuncDecl, pos token.Pos) bool {
	if decl.Body == nil {
		return false
	}
	var path []ast.Node
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if n == nil || pos < n.Pos() || pos >= n.End() {
			return false
		}
		path = append(path, n)
		return true
	})
	for i := len(path) - 1; i > 0; i-- {
		child, parent := path[i], path[i-1]
		switch p := parent.(type) {
		case *ast.IfStmt:
			if child == p.Body && z.implies(p.Cond, true) {
				return true
			}
			if p.Else != nil && child == p.Else && z.implies(p.Cond, false) {
				return true
			}
		case *ast.BinaryExpr:
			if child == p.Y && p.Op == token.LAND && z.implies(p.X, true) {
				return true
			}
			if child == p.Y && p.Op == token.LOR && z.implies(p.X, false) {
				return true
			}
		case *ast.BlockStmt:
			if z.leftBefore(p.List, child) {
				return true
			}
		case *ast.CaseClause:
			if z.leftBefore(p.Body, child) {
				return true
			}
		case *ast.CommClause:
			if z.leftBefore(p.Body, child) {
				return true
			}
		}
	}
	return false
}

func (z *zeroFields) leftBefore(list []ast.Stmt, child ast.Node) bool {
	for _, stmt := range list {
		if stmt == child {
			return false
		}
		ifs, ok := stmt.(*ast.IfStmt)
		if ok == false || ifs.Else != nil || z.leaves(ifs.Body) == false {
			continue
		}
		if z.implies(ifs.Cond, false) {
			return true
		}
	}
	return false
}

func (z *zeroFields) leaves(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}
	switch last := body.List[len(body.List)-1].(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BranchStmt:
		return last.Tok == token.BREAK || last.Tok == token.CONTINUE
	case *ast.ExprStmt:
		call, ok := ast.Unparen(last.X).(*ast.CallExpr)
		if ok == false {
			return false
		}
		id, ok := ast.Unparen(call.Fun).(*ast.Ident)
		if ok == false {
			return false
		}
		b, ok := z.w.pass.TypesInfo.Uses[id].(*types.Builtin)
		return ok && b.Name() == "panic"
	}
	return false
}

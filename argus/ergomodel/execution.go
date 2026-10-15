package ergomodel

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
)

var syncCallers = []struct {
	fn     string
	recv   string
	method string
	param  int
}{
	{fn: "sort.Slice", param: 1},
	{fn: "sort.SliceStable", param: 1},
	{fn: "sort.Search", param: 1},
	{fn: "slices.SortFunc", param: 1},
	{fn: "slices.SortStableFunc", param: 1},
	{fn: "slices.IndexFunc", param: 1},
	{fn: "slices.ContainsFunc", param: 1},
	{fn: "slices.DeleteFunc", param: 1},
	{recv: "sync.Map", method: "Range", param: 0},
	{recv: "sync.Once", method: "Do", param: 0},
}

func syncCallerParam(fn *types.Func) (int, bool) {
	recv := recvTypePath(fn)
	full := ""
	if fn.Pkg() != nil {
		full = fn.Pkg().Path() + "." + fn.Name()
	}
	for _, s := range syncCallers {
		if s.fn != "" && s.fn == full && recv == "" {
			return s.param, true
		}
		if s.method != "" && s.method == fn.Name() && s.recv == recv {
			return s.param, true
		}
	}
	return 0, false
}

func (m *Model) InspectExecuted(body *ast.BlockStmt, visit func(ast.Node) bool) {
	if body == nil {
		return
	}
	closures := m.executedClosures(body)
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			return false
		case *ast.FuncLit:
			if closures[x] == false {
				return false
			}
		}
		return visit(n)
	})
}

func (m *Model) executedClosures(body *ast.BlockStmt) map[*ast.FuncLit]bool {
	out := map[*ast.FuncLit]bool{}
	if body == nil {
		return out
	}
	info := m.pass.TypesInfo

	type binding struct {
		lit   *ast.FuncLit
		level *ast.FuncLit
	}
	type use struct {
		id    *ast.Ident
		level *ast.FuncLit
	}
	bound := map[types.Object]binding{}
	uses := map[types.Object][]use{}
	called := map[*ast.Ident]bool{}

	var stack []ast.Node
	var lits []*ast.FuncLit
	level := func() *ast.FuncLit {
		if len(lits) == 0 {
			return nil
		}
		return lits[len(lits)-1]
	}
	bind := func(id *ast.Ident, value ast.Expr) {
		lit, isLit := ast.Unparen(value).(*ast.FuncLit)
		if isLit == false {
			return
		}
		if obj := info.Defs[id]; obj != nil {
			bound[obj] = binding{lit: lit, level: level()}
		}
	}

	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if lit, isLit := top.(*ast.FuncLit); isLit && level() == lit {
				lits = lits[:len(lits)-1]
			}
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, n)

		switch x := n.(type) {
		case *ast.FuncLit:
			lits = append(lits, x)
		case *ast.CallExpr:
			if _, started := parent.(*ast.GoStmt); started {
				return true
			}
			switch fun := ast.Unparen(x.Fun).(type) {
			case *ast.FuncLit:
				out[fun] = true
			case *ast.Ident:
				called[fun] = true
			}
			if fn, ok := m.plainCallee(x); ok {
				if p, sync := syncCallerParam(fn); sync && p < len(x.Args) {
					if lit, isLit := ast.Unparen(x.Args[p]).(*ast.FuncLit); isLit {
						out[lit] = true
					}
				}
			}
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE && len(x.Lhs) == len(x.Rhs) {
				for i, lhs := range x.Lhs {
					if id, isIdent := lhs.(*ast.Ident); isIdent {
						bind(id, x.Rhs[i])
					}
				}
			}
		case *ast.ValueSpec:
			if len(x.Names) == len(x.Values) {
				for i, name := range x.Names {
					bind(name, x.Values[i])
				}
			}
		case *ast.Ident:
			if obj := info.Uses[x]; obj != nil {
				uses[obj] = append(uses[obj], use{id: x, level: level()})
			}
		}
		return true
	})

	for obj, b := range bound {
		if _, local := obj.(*types.Var); local == false {
			continue
		}
		inPlace := true
		for _, u := range uses[obj] {
			if called[u.id] == false || u.level != b.level {
				inPlace = false
				break
			}
		}
		if inPlace {
			out[b.lit] = true
		}
	}
	return out
}

func (m *Model) freshSends(body *ast.BlockStmt) map[*ast.SendStmt]bool {
	out := map[*ast.SendStmt]bool{}
	if body == nil {
		return out
	}
	info := m.pass.TypesInfo

	type channel struct {
		capacity int64
		level    *ast.FuncLit
		loops    int
	}
	type event struct {
		pos    token.Pos
		send   *ast.SendStmt
		looped bool
	}
	made := map[types.Object]*channel{}
	events := map[types.Object][]event{}

	var stack []ast.Node
	var lits []*ast.FuncLit
	var loops []ast.Node
	level := func() *ast.FuncLit {
		if len(lits) == 0 {
			return nil
		}
		return lits[len(lits)-1]
	}
	loopsAt := func(lit *ast.FuncLit) int {
		count := 0
		for _, loop := range loops {
			if lit == nil || (loop.Pos() >= lit.Pos() && loop.End() <= lit.End()) {
				count++
			}
		}
		return count
	}
	note := func(id *ast.Ident, value ast.Expr, once bool) {
		capacity, ok := m.bufferedMake(value)
		if ok == false {
			return
		}
		loops := loopsAt(level())
		if once {
			loops--
		}
		if obj := info.Defs[id]; obj != nil {
			made[obj] = &channel{capacity: capacity, level: level(), loops: loops}
		}
	}
	gotos := false

	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch top.(type) {
			case *ast.FuncLit:
				lits = lits[:len(lits)-1]
			case *ast.ForStmt, *ast.RangeStmt:
				loops = loops[:len(loops)-1]
			}
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, n)

		switch x := n.(type) {
		case *ast.FuncLit:
			lits = append(lits, x)
		case *ast.ForStmt, *ast.RangeStmt:
			loops = append(loops, x)
		case *ast.BranchStmt:
			if x.Tok == token.GOTO {
				gotos = true
			}
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE && len(x.Lhs) == len(x.Rhs) {

				loop, inInit := parent.(*ast.ForStmt)
				once := inInit && loop.Init == x
				for i, lhs := range x.Lhs {
					if id, isIdent := lhs.(*ast.Ident); isIdent {
						note(id, x.Rhs[i], once)
					}
				}
			}
		case *ast.ValueSpec:
			if len(x.Names) == len(x.Values) {
				for i, name := range x.Names {
					note(name, x.Values[i], false)
				}
			}
		case *ast.Ident:
			obj := info.Uses[x]
			ch, ok := made[obj]
			if ok == false {
				return true
			}
			at := level()
			if at != ch.level {

				events[obj] = append(events[obj], event{pos: capturedAt(lits, ch.level)})
				return true
			}
			switch p := parent.(type) {
			case *ast.SendStmt:
				if p.Chan == x {

					events[obj] = append(events[obj], event{
						pos: p.End(), send: p, looped: loopsAt(at) > ch.loops,
					})
					return true
				}
			case *ast.CallExpr:
				if isBuiltin(info, p.Fun, "len") || isBuiltin(info, p.Fun, "cap") {
					return true
				}
			}
			events[obj] = append(events[obj], event{pos: x.Pos()})
		}
		return true
	})
	if gotos {
		return out
	}

	for obj, list := range events {
		sort.Slice(list, func(i, j int) bool { return list[i].pos < list[j].pos })
		room := made[obj].capacity
		for _, e := range list {
			if e.send == nil || e.looped || room == 0 {
				break
			}
			out[e.send] = true
			room--
		}
	}
	return out
}

func capturedAt(lits []*ast.FuncLit, level *ast.FuncLit) token.Pos {
	if level == nil {
		return lits[0].Pos()
	}
	for i, lit := range lits {
		if lit == level && i+1 < len(lits) {
			return lits[i+1].Pos()
		}
	}
	return lits[len(lits)-1].Pos()
}

func (m *Model) bufferedMake(e ast.Expr) (int64, bool) {
	info := m.pass.TypesInfo
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if ok == false || len(call.Args) != 2 || isBuiltin(info, call.Fun, "make") == false {
		return 0, false
	}
	t := info.TypeOf(call.Args[0])
	if t == nil {
		return 0, false
	}
	if _, isChan := t.Underlying().(*types.Chan); isChan == false {
		return 0, false
	}
	tv, ok := info.Types[call.Args[1]]
	if ok == false || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	capacity, exact := constant.Int64Val(tv.Value)
	if exact == false || capacity < 1 {
		return 0, false
	}
	return capacity, true
}

func isBuiltin(info *types.Info, fun ast.Expr, name string) bool {
	id, ok := ast.Unparen(fun).(*ast.Ident)
	if ok == false {
		return false
	}
	builtin, ok := info.Uses[id].(*types.Builtin)
	return ok && builtin.Name() == name
}

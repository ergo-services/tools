package ergomodel

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
)

type ForkSite struct {
	Call      *ast.CallExpr
	Arg       ast.Expr
	Via       string
	What      string
	In        *Callback
	Enclosing *types.Func
	Decl      *ast.FuncDecl

	Resolved  bool
	Recovered bool
}

func (m *Model) ForkSites() []*ForkSite { return m.forkSites }

func (m *Model) buildForks(decls []*ast.FuncDecl, byDecl map[*ast.FuncDecl]*Callback) {
	m.forks = map[*types.Func][]int{}
	m.forkSitesIn = map[*ast.FuncDecl][]*ForkSite{}

	type unit struct {
		decl *ast.FuncDecl
		obj  *types.Func
	}
	var units []unit
	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		obj, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if obj == nil {
			continue
		}
		units = append(units, unit{decl: decl, obj: obj})
	}

	add := func(fn *types.Func, index int) bool {
		for _, have := range m.forks[fn] {
			if have == index {
				return false
			}
		}
		m.forks[fn] = append(m.forks[fn], index)
		return true
	}

	for changed := true; changed; {
		changed = false
		for _, u := range units {
			ast.Inspect(u.decl.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.GoStmt:
					if index, ok := m.funcParam(u.decl, x.Call.Fun); ok && add(u.obj, index) {
						changed = true
					}
				case *ast.CallExpr:
					fn, ok := m.plainCallee(x)
					if ok == false {
						return true
					}
					for _, p := range m.forkParams(fn) {
						if p >= len(x.Args) {
							continue
						}
						if index, ok := m.funcParam(u.decl, x.Args[p]); ok && add(u.obj, index) {
							changed = true
						}
					}
				}
				return true
			})
		}
	}

	for _, u := range units {
		cb := byDecl[u.decl]
		ast.Inspect(u.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, ok := m.plainCallee(call)
			if ok == false {
				return true
			}
			for _, p := range m.forkParams(fn) {
				if p >= len(call.Args) {
					continue
				}
				arg := call.Args[p]
				if _, forwarded := m.funcParam(u.decl, arg); forwarded {
					continue
				}
				site := &ForkSite{
					Call: call, Arg: arg, Via: m.forkName(fn),
					In: cb, Enclosing: u.obj, Decl: u.decl,
				}
				site.What, site.Resolved, site.Recovered = m.funcBoundary(arg)
				m.forkSites = append(m.forkSites, site)
				m.forkSitesIn[u.decl] = append(m.forkSitesIn[u.decl], site)
			}
			return true
		})
	}

	for _, u := range units {
		if params := m.forks[u.obj]; len(params) > 0 {
			m.pass.ExportObjectFact(u.obj, &ForksFact{Params: params})
		}
	}
}

func (m *Model) forkParams(fn *types.Func) []int {
	recv := recvTypePath(fn)
	full := ""
	if fn.Pkg() != nil {
		full = fn.Pkg().Path() + "." + fn.Name()
	}
	for _, s := range m.cfg.Forks {
		if s.Func != "" && s.Func == full && recv == "" {
			return []int{s.Param}
		}
		if s.Method != "" && s.Method == fn.Name() && s.Recv == recv {
			return []int{s.Param}
		}
	}
	if fn.Pkg() == m.pass.Pkg {
		return m.forks[fn]
	}
	var fact ForksFact
	if m.pass.ImportObjectFact(fn, &fact) {
		return fact.Params
	}
	return nil
}

func (m *Model) funcParam(decl *ast.FuncDecl, e ast.Expr) (int, bool) {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if ok == false || decl.Type.Params == nil {
		return 0, false
	}
	obj := m.pass.TypesInfo.Uses[id]
	if obj == nil {
		return 0, false
	}
	if _, isFunc := obj.Type().Underlying().(*types.Signature); isFunc == false {
		return 0, false
	}
	index := 0
	for _, field := range decl.Type.Params.List {
		for _, name := range field.Names {
			if m.pass.TypesInfo.Defs[name] == obj {
				return index, true
			}
			index++
		}
		if len(field.Names) == 0 {
			index++
		}
	}
	return 0, false
}

func (m *Model) funcBoundary(e ast.Expr) (what string, resolved bool, recovered bool) {
	if lit, ok := ast.Unparen(e).(*ast.FuncLit); ok {
		return "this func literal", true, m.establishesBoundary(lit.Body)
	}
	fn := m.funcValue(ast.Unparen(e))
	if fn == nil {
		return "this func value", false, false
	}
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
		return fn.Name(), false, false
	}
	return fn.Name(), true, m.funcRecovers(fn)
}

func (m *Model) funcRecovers(fn *types.Func) bool {
	if fn.Pkg() == m.pass.Pkg {
		if decl, ok := m.declOf[fn]; ok {
			return m.establishesBoundary(decl.Body)
		}
		return false
	}
	var f RecoversFact
	return m.pass.ImportObjectFact(fn, &f)
}

func (m *Model) forkName(fn *types.Func) string {
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		if named, ok := deref(sig.Recv().Type()).(*types.Named); ok {
			return named.Obj().Name() + "." + fn.Name()
		}
	}
	if fn.Pkg() != nil && fn.Pkg() != m.pass.Pkg {
		return fn.Pkg().Name() + "." + fn.Name()
	}
	return fn.Name()
}

func (m *Model) noteUnrecovered(out *FuncBehavior, obj *types.Func, pos token.Pos) {
	if out.Unrecovered || m.Suppressed(pos, "A1010") {
		return
	}
	at := m.fset.Position(pos)
	out.Unrecovered = true
	out.UnrecoveredIn = FuncID(obj)
	out.UnrecoveredAt = fmt.Sprintf("%s:%d", filepath.Base(at.Filename), at.Line)
}

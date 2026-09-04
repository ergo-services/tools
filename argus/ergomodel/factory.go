package ergomodel

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

type FactoryInfo struct {
	Behavior string

	InitBlocks  bool
	InitWhy     string
	InitTimeout int
	Chain       string

	Sentinel string
}

func (m *Model) Factories() map[types.Object]FactoryInfo { return m.factories }

func (m *Model) FactoryOf(e ast.Expr) (types.Object, FactoryInfo, bool) {
	obj := m.factoryObject(e)
	if obj == nil {
		return nil, FactoryInfo{}, false
	}
	if info, ok := m.factories[obj]; ok {
		return obj, info, true
	}

	var info FactoryInfo
	found := false
	var link FactoryFact
	if m.pass.ImportObjectFact(obj, &link) {
		info.Behavior, found = link.Behavior, true
	}
	var budget InitBudgetFact
	if m.pass.ImportObjectFact(obj, &budget) {
		info.InitBlocks, info.InitWhy = budget.Blocks, budget.Why
		info.InitTimeout, info.Chain = budget.InnerTimeout, budget.Chain
		found = true
	}
	var sentinel InitSentinelFact
	if m.pass.ImportObjectFact(obj, &sentinel) {
		info.Sentinel, found = sentinel.Sentinel, true
	}
	return obj, info, found
}

func (m *Model) factoryObject(e ast.Expr) types.Object {
	switch x := e.(type) {
	case *ast.Ident:
		if obj := m.pass.TypesInfo.Uses[x]; obj != nil {
			return obj
		}
		return m.pass.TypesInfo.Defs[x]
	case *ast.SelectorExpr:
		return m.pass.TypesInfo.Uses[x.Sel]
	}
	return nil
}

const factoryResultPath = "ergo.services/ergo/gen.ProcessBehavior"

func (m *Model) buildFactories(decls []*ast.FuncDecl) {
	m.factories = map[types.Object]FactoryInfo{}

	inits := map[string]*ast.FuncDecl{}
	for _, cb := range m.Callbacks {
		if cb.Kind == CBInit && cb.Meta == false {
			inits[typePath(cb.Behavior)] = cb.Decl
		}
	}

	record := func(obj types.Object, behavior string, pos token.Pos) {
		info := FactoryInfo{Behavior: behavior}
		if decl, ok := inits[behavior]; ok {
			info.InitBlocks, info.InitWhy, info.InitTimeout, info.Chain = m.initBudget(decl)
			info.Sentinel = m.initSentinel(decl)
		}
		m.factories[obj] = info
		m.publishFactory(obj, info)
	}

	for _, decl := range decls {
		if decl.Recv != nil || decl.Body == nil {
			continue
		}
		obj, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
		if obj == nil || returnsProcessBehavior(obj.Type()) == false {
			continue
		}
		if behavior := m.returnedBehavior(decl.Body); behavior != "" {
			record(obj, behavior, decl.Pos())
		}
	}

	for _, file := range m.pass.Files {
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if ok == false || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if ok == false {
					continue
				}
				for i, name := range vs.Names {
					obj := m.pass.TypesInfo.Defs[name]
					if obj == nil || returnsProcessBehavior(obj.Type()) == false {
						continue
					}
					if i >= len(vs.Values) {
						continue
					}

					switch v := vs.Values[i].(type) {
					case *ast.FuncLit:
						if behavior := m.returnedBehavior(v.Body); behavior != "" {
							record(obj, behavior, name.Pos())
						}
					case *ast.Ident:
						if target := m.factoryObject(v); target != nil {
							if info, ok := m.factories[target]; ok {
								m.factories[obj] = info
								m.publishFactory(obj, info)
							}
						}
					}
				}
			}
		}
	}
}

func returnsProcessBehavior(t types.Type) bool {
	sig, ok := t.Underlying().(*types.Signature)
	if ok == false || sig.Results().Len() != 1 {
		return false
	}
	named, ok := sig.Results().At(0).Type().(*types.Named)
	if ok == false {
		return false
	}
	return typePath(named) == factoryResultPath
}

func (m *Model) returnedBehavior(body *ast.BlockStmt) string {
	found := ""
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		t := m.pass.TypesInfo.TypeOf(ret.Results[0])
		if t == nil {
			return true
		}
		named, ok := deref(t).(*types.Named)
		if ok == false {
			return true
		}
		path := typePath(named)
		if found != "" && found != path {
			found = ""
			return false
		}
		found = path
		return true
	})
	return found
}

func (m *Model) initBudget(decl *ast.FuncDecl) (blocks bool, why string, timeout int, chain string) {
	obj, _ := m.pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if obj == nil {
		return false, "", 0, ""
	}
	b := m.Behavior(obj)
	if b.Blocks == false {
		return false, "", 0, ""
	}
	return true, b.Why, b.Timeout, FuncID(obj)
}

func (m *Model) initSentinel(decl *ast.FuncDecl) string {
	found := ""
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found != "" {
			return false
		}

		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		if name := m.frameworkSentinel(ret.Results[0]); name != "" {
			found = name
			return false
		}
		return true
	})
	return found
}

func (m *Model) frameworkSentinel(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false {
		return ""
	}
	obj := m.pass.TypesInfo.Uses[sel.Sel]
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	if obj.Pkg().Path() != "ergo.services/ergo/gen" {
		return ""
	}
	name := sel.Sel.Name
	if strings.HasPrefix(name, "TerminateReason") || strings.HasPrefix(name, "Err") {
		return "gen." + name
	}
	return ""
}

func (m *Model) publishFactory(obj types.Object, info FactoryInfo) {
	if info.Behavior != "" {
		m.pass.ExportObjectFact(obj, &FactoryFact{Behavior: info.Behavior})
	}
	if info.InitBlocks {
		m.pass.ExportObjectFact(obj, &InitBudgetFact{
			Blocks: true, Why: info.InitWhy,
			InnerTimeout: info.InitTimeout, Chain: info.Chain,
		})
	}
	if info.Sentinel != "" {
		m.pass.ExportObjectFact(obj, &InitSentinelFact{Sentinel: info.Sentinel})
	}
}

type SpawnPoint struct {
	Pos     token.Pos
	Factory ast.Expr
	Timeout FieldValue
	Capped  bool
	What    string
	In      *ast.FuncDecl
}

func (m *Model) SpawnPoints() []*SpawnPoint { return m.spawnPoints }

var factoryArg = map[string]int{
	"Spawn":         0,
	"SpawnRegister": 1,
}

func (m *Model) buildSpawnPoints(decls []*ast.FuncDecl) {
	for _, decl := range decls {
		if decl.Body == nil {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			fn, ok := m.calleeFunc(call)
			if ok == false || fn.Pkg() == nil {
				return true
			}
			if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
				return true
			}
			idx, ok := factoryArg[fn.Name()]
			if ok == false || idx >= len(call.Args) {
				return true
			}
			point := &SpawnPoint{
				Pos: call.Pos(), Factory: call.Args[idx],
				What: fn.Name(), In: decl,
			}

			if idx+1 < len(call.Args) {
				opts := &SupervisorSpec{fields: map[string]FieldValue{}}
				m.foldLiteral(opts, call.Args[idx+1], "")
				point.Timeout = opts.fields["InitTimeout"]
			}
			m.spawnPoints = append(m.spawnPoints, point)
			return true
		})
	}

	for _, spec := range m.specs {
		if spec.Resolved == false || spec.ChildrenUnresolved {
			continue
		}
		for i, child := range spec.Children {
			factory := child["Factory"]
			if factory.Expr == nil {
				continue
			}
			m.spawnPoints = append(m.spawnPoints, &SpawnPoint{
				Pos: spec.ChildrenPos[i], Factory: factory.Expr,
				Timeout: child["Options.InitTimeout"],
				What:    "supervisor child spec", In: spec.Decl,
			})
		}
	}

	for _, member := range m.appMembers {
		if member.Factory.Expr == nil {
			continue
		}
		m.spawnPoints = append(m.spawnPoints, &SpawnPoint{
			Pos: member.Pos, Factory: member.Factory.Expr,
			Timeout: member.InitTimeout, Capped: true,
			What: "application group member", In: member.In,
		})
	}
}

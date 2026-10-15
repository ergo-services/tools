package initsentinel

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/astutil"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2017"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2017",
	Doc: `A2017: Init returning a framework sentinel as control flow.

Init's error is the spawn failure channel, not a termination reason. Returning
gen.TerminateReasonNormal from Init to mean "another process already owns this name"
invents a convention, and then every caller has to know it: the spawn reports an error,
the error is a sentinel that means success, and whoever forgets to decode it logs a
failure that did not happen.

On a production corpus twelve Init implementations did this, and two sibling supervisors
disagreed about decoding it, so one of them produced error logs and inflated failure
counters on an ordinary duplicate spawn.

Two halves. At the declaration: an Init whose error result is a framework sentinel, when
a spawn point of its factory in the same package does not decode it, or when no spawn
point is visible there to show that its spawners do. An Init whose every visible spawner
decodes the sentinel is a convention the package keeps, and is not reported. At the spawn
site: a factory whose Init signals through a sentinel, spawned by code that never mentions
that sentinel, which is the caller that will misread a normal outcome as a failure. The
second half is why the verdict travels on the factory object, and it is what covers the
spawners in other packages.

A child of a SimpleOneForOne supervisor is not started by its spec, which is only a
template: it is started by StartChild, so the StartChild calls of that supervisor are the
spawn points, and the function holding each one is where the sentinel has to be decoded.

One shape is reported whatever the spawners do: an Init that returns the sentinel when it
rejects its own arguments. That is a spawn failure disguised as a normal outcome, and a
spawner that decodes the sentinel as benign is exactly the one that hides it.

The fix is to make the outcome explicit rather than encoded: let Init succeed and have
the process decide what to do, or return a distinct error type the caller matches on
deliberately, and refuse bad arguments with a real error such as gen.ErrIncorrect.`,
	URL:      "https://docs.ergo.services/tools/argus#A2017",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const (
	ergoPrefix          = "ergo.services/ergo"
	simpleOneForOne     = 3
	supervisorChildSpec = "supervisor child spec"
)

type spawner struct {
	pos  token.Pos
	what string
	in   *ast.FuncDecl
}

type checker struct {
	pass   *analysis.Pass
	m      *ergomodel.Model
	starts map[*types.Named][]startCall
}

type startCall struct {
	call *ast.CallExpr
	in   *ast.FuncDecl
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)
	c := &checker{pass: pass, m: m, starts: startChildCalls(pass)}

	for _, cb := range m.Callbacks {
		if cb.Kind != ergomodel.CBInit || cb.Meta {
			continue
		}
		if sentinel, pos, ok := c.rejectsArgs(cb.Decl); ok {
			m.Report(pass, pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: ergomodel.CallbackID(cb), Witness: sentinel,
				},
				"Init returns %s when it rejects its arguments, which disguises a spawn failure as a normal outcome, so a spawner that decodes the sentinel as benign never sees the failure; refuse with a real error such as gen.ErrIncorrect",
				sentinel)
			continue
		}

		sentinel, via, pos := m.SentinelReturn(cb.Decl)
		if sentinel == "" {
			continue
		}
		points := c.spawnersOf(cb.Behavior)
		var blind *spawner
		for _, p := range points {
			if mentions(pass, p.in, sentinel) == false {
				blind = p
				break
			}
		}
		if len(points) > 0 && blind == nil {
			continue
		}

		why := "so every spawner has to decode this particular error to tell it from a spawn failure, and no spawn point in this package shows that they do"
		if blind != nil {
			why = "and the " + blind.what + " in " + funcLabel(pass, blind.in) + " does not decode it, so that spawner cannot tell this outcome from a spawn failure"
		}
		m.Report(pass, pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(cb), Witness: sentinel,
			},
			"Init returns %s%s as its error, which is the spawn failure channel rather than a termination reason, %s",
			sentinel, through(via), why)
	}

	for _, point := range m.SpawnPoints() {
		if point.Factory == nil || point.In == nil {
			continue
		}
		obj, info, ok := m.FactoryOf(point.Factory)
		if ok == false || info.Sentinel == "" {
			continue
		}

		if m.Factories()[obj].Sentinel != "" {
			continue
		}
		for _, s := range c.expand(point) {
			if mentions(pass, s.in, info.Sentinel) {
				continue
			}
			m.Report(pass, s.pos,
				ergomodel.Finding{
					Rule: ruleID, Kind: ergomodel.KindSpec, Tier: 2,
					ID:      m.DeclID(s.in) + ":" + obj.Name(),
					Witness: info.Sentinel,
				},
				"%s signals a normal outcome by returning %s from Init, and this %s never mentions it, so an ordinary outcome is reported here as a spawn failure",
				obj.Name(), info.Sentinel, s.what)
		}
	}
	return nil, nil
}

func (c *checker) spawnersOf(behavior *types.Named) []*spawner {
	want := ergomodel.TypeID(behavior)
	var out []*spawner
	for _, point := range c.m.SpawnPoints() {
		if point.Factory == nil || point.In == nil || c.builds(point.Factory) != want {
			continue
		}
		out = append(out, c.expand(point)...)
	}
	return out
}

func (c *checker) builds(factory ast.Expr) string {
	if lit, ok := ast.Unparen(factory).(*ast.FuncLit); ok {
		return c.literalBehavior(lit)
	}
	_, info, ok := c.m.FactoryOf(factory)
	if ok == false {
		return ""
	}
	return info.Behavior
}

func (c *checker) literalBehavior(lit *ast.FuncLit) string {
	found := ""
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		t := c.pass.TypesInfo.TypeOf(ret.Results[0])
		if t == nil {
			return true
		}
		if ptr, isPtr := t.(*types.Pointer); isPtr {
			t = ptr.Elem()
		}
		named, ok := t.(*types.Named)
		if ok == false {
			return true
		}
		found = ergomodel.TypeID(named)
		return false
	})
	return found
}

func (c *checker) expand(point *ergomodel.SpawnPoint) []*spawner {
	if point.What == supervisorChildSpec {
		if spec, child := c.specOf(point); spec != nil {
			if t := spec.Field("Type"); t.HasInt && t.Int == simpleOneForOne {
				return c.startersOf(spec, child)
			}
		}
	}
	return []*spawner{{pos: point.Pos, what: point.What, in: point.In}}
}

func (c *checker) specOf(point *ergomodel.SpawnPoint) (*ergomodel.SupervisorSpec, map[string]ergomodel.FieldValue) {
	for _, spec := range c.m.Specs() {
		for i, pos := range spec.ChildrenPos {
			if pos == point.Pos && i < len(spec.Children) {
				return spec, spec.Children[i]
			}
		}
	}
	return nil, nil
}

func (c *checker) startersOf(spec *ergomodel.SupervisorSpec, child map[string]ergomodel.FieldValue) []*spawner {
	fn, ok := c.pass.TypesInfo.Defs[spec.Decl.Name].(*types.Func)
	if ok == false {
		return nil
	}
	sup := ownerOf(fn)
	if sup == nil {
		return nil
	}
	name := child["Name"]
	var out []*spawner
	for _, s := range c.starts[sup] {
		tv := c.pass.TypesInfo.Types[s.call.Args[0]]
		if name.HasStr && tv.Value != nil && tv.Value.Kind() == constant.String &&
			constant.StringVal(tv.Value) != name.Str {
			continue
		}
		out = append(out, &spawner{pos: s.call.Pos(), what: "StartChild", in: s.in})
	}
	return out
}

func startChildCalls(pass *analysis.Pass) map[*types.Named][]startCall {
	out := map[*types.Named][]startCall{}
	for _, file := range pass.Files {
		for _, d := range file.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if ok == false || decl.Body == nil {
				continue
			}
			ast.Inspect(decl.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok == false || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok == false || sel.Sel.Name != "StartChild" {
					return true
				}
				fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
				if ok == false || fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
					return true
				}
				t := pass.TypesInfo.TypeOf(sel.X)
				if ptr, isPtr := t.(*types.Pointer); isPtr {
					t = ptr.Elem()
				}
				if named, isNamed := t.(*types.Named); isNamed {
					out[named] = append(out[named], startCall{call: call, in: decl})
				}
				return true
			})
		}
	}
	return out
}

func (c *checker) rejectsArgs(decl *ast.FuncDecl) (string, token.Pos, bool) {
	if decl == nil || decl.Body == nil || decl.Type.Params == nil || len(decl.Type.Params.List) == 0 {
		return "", token.NoPos, false
	}
	names := decl.Type.Params.List[0].Names
	if len(names) == 0 {
		return "", token.NoPos, false
	}
	args := c.pass.TypesInfo.Defs[names[0]]
	if args == nil {
		return "", token.NoPos, false
	}
	file := c.fileOf(decl)
	if file == nil {
		return "", token.NoPos, false
	}
	defs := localDefinitions(c.pass.TypesInfo, decl.Body)

	sentinel, at := "", token.NoPos
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if sentinel != "" {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if ok == false || len(ret.Results) != 1 {
			return true
		}
		name := terminateReason(c.pass.TypesInfo, ret.Results[0])
		if name == "" {
			return true
		}
		path, _ := astutil.PathEnclosingInterval(file, ret.Pos(), ret.End())
		for _, node := range path {
			for _, cond := range guards(node) {
				if cond != nil && refersTo(c.pass.TypesInfo, cond, args, defs, 0) {
					sentinel, at = name, ret.Pos()
					return false
				}
			}
		}
		return true
	})
	return sentinel, at, sentinel != ""
}

func (c *checker) fileOf(decl *ast.FuncDecl) *ast.File {
	for _, file := range c.pass.Files {
		if file.Pos() <= decl.Pos() && decl.End() <= file.End() {
			return file
		}
	}
	return nil
}

func guards(n ast.Node) []ast.Node {
	var out []ast.Node
	switch x := n.(type) {
	case *ast.IfStmt:
		out = append(out, x.Init, x.Cond)
	case *ast.SwitchStmt:
		out = append(out, x.Init, x.Tag)
	case *ast.TypeSwitchStmt:
		out = append(out, x.Init, x.Assign)
	case *ast.CaseClause:
		for _, e := range x.List {
			out = append(out, e)
		}
	}
	return out
}

func localDefinitions(info *types.Info, body *ast.BlockStmt) map[types.Object][]ast.Expr {
	out := map[types.Object][]ast.Expr{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				id, ok := lhs.(*ast.Ident)
				if ok == false {
					continue
				}
				obj := info.ObjectOf(id)
				if obj == nil {
					continue
				}
				if len(x.Rhs) == len(x.Lhs) {
					out[obj] = append(out[obj], x.Rhs[i])
					continue
				}
				out[obj] = append(out[obj], x.Rhs...)
			}
		case *ast.ValueSpec:
			for i, id := range x.Names {
				obj := info.ObjectOf(id)
				if obj == nil {
					continue
				}
				if len(x.Values) == len(x.Names) {
					out[obj] = append(out[obj], x.Values[i])
					continue
				}
				out[obj] = append(out[obj], x.Values...)
			}
		}
		return true
	})
	return out
}

func refersTo(info *types.Info, n ast.Node, target types.Object, defs map[types.Object][]ast.Expr, depth int) bool {
	if depth > 3 {
		return false
	}
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		if found {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			return transparent(info, call)
		}
		id, ok := node.(*ast.Ident)
		if ok == false {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			return true
		}
		if obj == target {
			found = true
			return false
		}
		for _, e := range defs[obj] {
			if refersTo(info, e, target, defs, depth+1) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func transparent(info *types.Info, call *ast.CallExpr) bool {
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() {
		return true
	}
	if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok {
		_, builtin := info.Uses[id].(*types.Builtin)
		return builtin
	}
	return false
}

func terminateReason(info *types.Info, e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if ok == false {
		return ""
	}
	obj := info.Uses[sel.Sel]
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != ergoPrefix+"/gen" {
		return ""
	}
	if strings.HasPrefix(sel.Sel.Name, "TerminateReason") == false {
		return ""
	}
	return "gen." + sel.Sel.Name
}

func funcLabel(pass *analysis.Pass, decl *ast.FuncDecl) string {
	fn, ok := pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if ok == false {
		return decl.Name.Name
	}
	if owner := ownerOf(fn); owner != nil {
		return owner.Obj().Name() + "." + fn.Name()
	}
	return fn.Name()
}

func ownerOf(fn *types.Func) *types.Named {
	sig, ok := fn.Type().(*types.Signature)
	if ok == false || sig.Recv() == nil {
		return nil
	}
	t := sig.Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok == true {
		t = ptr.Elem()
	}
	named, _ := t.(*types.Named)
	return named
}

func through(via string) string {
	if via == "" {
		return ""
	}
	return " through " + via
}

func mentions(pass *analysis.Pass, decl *ast.FuncDecl, sentinel string) bool {
	want := strings.TrimPrefix(sentinel, "gen.")
	found := false
	ast.Inspect(decl, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := n.(*ast.SelectorExpr)
		if ok == false || sel.Sel.Name != want {
			return true
		}
		obj := pass.TypesInfo.Uses[sel.Sel]
		if obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "ergo.services/ergo/gen" {
			found = true
		}
		return true
	})
	return found
}

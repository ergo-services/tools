package registration

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2009"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2009",
	Doc: `A2009: a type registration that fails at startup.

EDF registration is strict and every one of these is a startup error rather than a
degraded mode, so a node either refuses to start or refuses to talk to a peer:

  - RegisterType(&T{}) is rejected outright, since a pointer type is not supported.
    Register the value: RegisterType(T{}).
  - A concrete error type is rejected outright: an error has no structural encoding
    on the wire. Register the sentinel markers with RegisterError instead, and carry
    per-instance data in a typed field beside the error.
  - A bare bool, string, integer, float or []byte is rejected as "a regular type",
    and so is one of the framework's own value types, which edf registers itself.
  - A MarshalEDF without a pointer-receiver UnmarshalEDF is rejected with "must be a
    method of *T", and a value-receiver UnmarshalEDF is rejected for the same reason.
    The pair has to be Marshal on the value and Unmarshal on the pointer.
  - Registration performed outside an application Load, an ApplicationSpec.Network
    block or an init runs after the point where a peer may already have connected,
    so the two nodes disagree about the type table.
  - An application group member with Options.InitTimeout above 15 seconds. The
    framework does not clamp it: it refuses to start the whole application.

Placement is judged where the registration runs. A process callback (an actor's
Init, a handler, a meta callback, an application's Init or Start) is judged in place,
and so is main for a registration through a node, which it can only have after
StartNode; a package level registration in main may come before StartNode and is not
judged there. Any other function or method that registers, directly or through another
one, is a helper: it is not reported itself, it carries a fact, and each call to it is
judged by the function it is called from, in this package or in another. So an
exported RegisterTypes(node) called from Load is quiet, and the same helper called
from an actor's Init is reported at that call. A helper that is only ever passed
around as a value and never called is not judged at all, and nothing in the framework
is a helper: StartNode filling the edf table is not a registration its caller placed.

One finding is a hint rather than a failure, at tier 3: the package level edf entry
points (RegisterTypeOf, RegisterTypesOf, RegisterError, RegisterAtom). Called from
init they work, and nothing fails at startup; on a node that speaks only ENP,
Network().RegisterType is the same call into the same table. But they are deprecated,
the process prints one deprecation line on stderr for each of them on first use, and
they register with EDF alone, so on a node that runs another wire protocol beside ENP
only EDF learns the type. The replacement is ApplicationSpec.Network, or
Network().RegisterType in an application Load. A pointer or a refused type handed to
them is still a tier 2 startup failure, and so is registering a type through them
where a peer may already have connected.

Source: edf.md`,
	URL:       "https://docs.ergo.services/tools/argus#A2009",
	Requires:  []*analysis.Analyzer{ergomodel.Analyzer},
	FactTypes: []analysis.Fact{(*RegistersFact)(nil)},
	Run:       run,
}

type RegistersFact struct {
	Call string
	Node bool
}

func (*RegistersFact) AFact() {}

func (f *RegistersFact) String() string { return "registers through " + f.Call }

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	checkCalls(pass, m)
	checkMarshalerPairs(pass, m)
	checkInitTimeout(pass, m)
	return nil, nil
}

const initTimeoutCap = 15

func checkInitTimeout(pass *analysis.Pass, m *ergomodel.Model) {
	for _, member := range m.AppMembers() {
		timeout := member.InitTimeout
		if timeout.HasInt == false || timeout.Int <= initTimeoutCap {
			continue
		}
		name := member.Name.Str
		if name == "" {
			name = "this group member"
		}
		m.Report(pass, timeout.Pos,
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
				ID:      m.DeclID(member.In) + ":InitTimeout/" + name,
				Witness: "InitTimeout"},
			"Options.InitTimeout is %d on application group member %q, above the ceiling of %d seconds; the framework does not shorten it, it aborts the whole application start with ErrNotAllowed",
			timeout.Int, name, initTimeoutCap)
	}
}

type placement int

const (
	placedEarly placement = iota
	placedLate
	placedMain
	placedHelper
)

func (p placement) late(node bool) bool {
	return p == placedLate || (p == placedMain && node)
}

func checkCalls(pass *analysis.Pass, m *ergomodel.Model) {
	callbacks := map[*ast.FuncDecl]bool{}
	for _, cb := range m.Callbacks {
		callbacks[cb.Decl] = true
	}
	var decls []*ast.FuncDecl
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				decls = append(decls, fn)
			}
		}
	}

	registers := map[*types.Func]RegistersFact{}
	path := pass.Pkg.Path()
	framework := path == "ergo.services/ergo" || strings.HasPrefix(path, "ergo.services/ergo/")
	for changed := framework == false; changed; {
		changed = false
		for _, fn := range decls {
			obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if ok == false || placementOf(pass, callbacks, fn) != placedHelper {
				continue
			}
			fact, ok := registrationIn(pass, registers, fn)
			if ok && fact != registers[obj] {
				registers[obj] = fact
				changed = true
			}
		}
	}
	for obj, fact := range registers {
		pass.ExportObjectFact(obj, &RegistersFact{Call: fact.Call, Node: fact.Node})
	}

	for _, fn := range decls {
		placed := placementOf(pass, callbacks, fn)

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if ok == false {
				return true
			}
			if helper, fact, ok := registeringHelper(pass, registers, call); ok {
				if placed.late(fact.Node) {
					m.Report(pass, call.Pos(),
						callFinding(m, fn, "helper/"+helper.Name()+"/placement"),
						"%s, called from %s, registers types through %s after a peer may already have connected; call it from an application Load, list the types in ApplicationSpec.Network, or register in init",
						helper.Name(), describe(fn), fact.Call)
				}
				return true
			}
			callee, pkg, ok := ergoCallee(pass, call)
			if ok == false {
				return true
			}
			late := placed.late(pkg != edfPath)

			switch callee.Name() {
			case "RegisterType":
				reportPointerArgs(pass, m, call, 0)
				reportRefusedArgs(pass, m, call, 0)
				if late {
					m.Report(pass, call.Pos(),
						callFinding(m, fn, "RegisterType/placement"),
						"RegisterType in %s runs after a peer may already have connected; register in an application Load, in ApplicationSpec.Network or in init",
						describe(fn))
				}
			case "RegisterTypes":
				reportSliceArgs(pass, m, call)
				if late {
					m.Report(pass, call.Pos(),
						callFinding(m, fn, "RegisterTypes/placement"),
						"RegisterTypes in %s runs after a peer may already have connected; register in an application Load, in ApplicationSpec.Network or in init",
						describe(fn))
				}
			case "RegisterTypeOf", "RegisterTypesOf", "RegisterError", "RegisterAtom":
				if pkg != edfPath {
					break
				}
				switch callee.Name() {
				case "RegisterTypeOf":
					reportPointerArgs(pass, m, call, 0)
					reportRefusedArgs(pass, m, call, 0)
				case "RegisterTypesOf":
					reportSliceArgs(pass, m, call)
				}
				legacy := legacyEntries[callee.Name()]
				if late && legacy.types {
					m.Report(pass, call.Pos(),
						callFinding(m, fn, "edf/"+callee.Name()+"/placement"),
						"edf.%s in %s runs after a peer may already have connected; register in an application Load, in ApplicationSpec.Network or in init",
						callee.Name(), describe(fn))
				}
				deprecated := callFinding(m, fn, "edf/"+callee.Name())
				if callee.Name() == "RegisterTypeOf" {
					deprecated = callFinding(m, fn, "RegisterTypeOf")
				}
				deprecated.Tier = 3
				m.Report(pass, call.Pos(), deprecated,
					"edf.%s is deprecated and prints a deprecation notice on first use; it registers with EDF alone, so on a node that runs another wire protocol beside ENP only EDF knows the %s; list it in ApplicationSpec.Network.%s, or call Network().%s in an application Load",
					callee.Name(), legacy.noun, legacy.field, legacy.replacement)
			}
			return true
		})
	}
}

const edfPath = "ergo.services/ergo/net/edf"

var legacyEntries = map[string]struct {
	noun, field, replacement string
	types                    bool
}{
	"RegisterTypeOf":  {noun: "type", field: "RegisterTypes", replacement: "RegisterType", types: true},
	"RegisterTypesOf": {noun: "types", field: "RegisterTypes", replacement: "RegisterTypes", types: true},
	"RegisterError":   {noun: "error", field: "RegisterErrors", replacement: "RegisterError"},
	"RegisterAtom":    {noun: "atom", field: "RegisterAtoms", replacement: "RegisterAtom"},
}

func ergoCallee(pass *analysis.Pass, call *ast.CallExpr) (*types.Func, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return nil, "", false
	}
	callee, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if ok == false || callee.Pkg() == nil {
		return nil, "", false
	}
	pkg := callee.Pkg().Path()
	if strings.HasPrefix(pkg, "ergo.services/ergo/") == false {
		return nil, "", false
	}
	return callee, pkg, true
}

func registersTypes(pass *analysis.Pass, call *ast.CallExpr) (RegistersFact, bool) {
	callee, pkg, ok := ergoCallee(pass, call)
	if ok == false {
		return RegistersFact{}, false
	}
	switch callee.Name() {
	case "RegisterType", "RegisterTypes":
		if pkg == edfPath {
			return RegistersFact{Call: "edf." + callee.Name()}, true
		}
		return RegistersFact{Call: "Network()." + callee.Name(), Node: true}, true
	case "RegisterTypeOf", "RegisterTypesOf":
		if pkg == edfPath {
			return RegistersFact{Call: "edf." + callee.Name()}, true
		}
	}
	return RegistersFact{}, false
}

func registeringHelper(pass *analysis.Pass, local map[*types.Func]RegistersFact, call *ast.CallExpr) (*types.Func, RegistersFact, bool) {
	var id *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = fun
	case *ast.SelectorExpr:
		id = fun.Sel
	default:
		return nil, RegistersFact{}, false
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if ok == false {
		return nil, RegistersFact{}, false
	}
	fn = fn.Origin()
	if fact, ok := local[fn]; ok {
		return fn, fact, true
	}
	if fn.Pkg() == nil || fn.Pkg() == pass.Pkg {
		return nil, RegistersFact{}, false
	}
	var fact RegistersFact
	if pass.ImportObjectFact(fn, &fact) {
		return fn, fact, true
	}
	return nil, RegistersFact{}, false
}

func registrationIn(pass *analysis.Pass, local map[*types.Func]RegistersFact, fn *ast.FuncDecl) (RegistersFact, bool) {
	var found RegistersFact
	ok := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found.Node {
			return false
		}
		call, isCall := n.(*ast.CallExpr)
		if isCall == false {
			return true
		}
		fact, registers := registersTypes(pass, call)
		if registers == false {
			_, fact, registers = registeringHelper(pass, local, call)
		}
		if registers && (ok == false || fact.Node) {
			found, ok = fact, true
		}
		return true
	})
	return found, ok
}

func reportPointerArgs(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr, from int) {
	for i := from; i < len(call.Args); i++ {
		arg := call.Args[i]
		t := pass.TypesInfo.TypeOf(arg)
		if t == nil {
			continue
		}
		if _, isPtr := types.Unalias(t).(*types.Pointer); isPtr == false {
			continue
		}
		m.Report(pass, arg.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
				ID: ergomodel.TypeID(t), Witness: "pointer registration"},
			"a pointer type is not supported by registration; register the value instead of %s",
			exprString(pass, arg))
	}
}

func reportRefusedArgs(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr, from int) {
	for i := from; i < len(call.Args); i++ {
		reportRefused(pass, m, call.Args[i])
	}
}

func reportRefused(pass *analysis.Pass, m *ergomodel.Model, arg ast.Expr) {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return
	}
	u := types.Unalias(t)
	if _, isPtr := u.(*types.Pointer); isPtr {
		return
	}
	why, refused := refusedType(u)
	if refused == false {
		return
	}
	m.Report(pass, arg.Pos(),
		ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
			ID: ergomodel.TypeID(t), Witness: why},
		"registration refuses %s: %s",
		exprString(pass, arg), why)
}

func refusedType(t types.Type) (string, bool) {
	named, isNamed := t.(*types.Named)
	if isNamed == false {
		if isErrorIface(t) {
			return "an error carries no structural encoding, so register the sentinel markers with RegisterError instead", true
		}
		if _, isBasic := t.(*types.Basic); isBasic {
			return "a bare builtin type is registered by edf already, so this answers \"unable to register a regular type\"", true
		}
		if s, isSlice := t.(*types.Slice); isSlice {
			if b, isBasic := types.Unalias(s.Elem()).(*types.Basic); isBasic && b.Kind() == types.Uint8 {
				return "[]byte is registered by edf already, so this answers \"unable to register a regular type\"", true
			}
		}
		return "", false
	}
	if isErrorIface(named) {
		return "a concrete error type carries no structural encoding, so register the sentinel markers with RegisterError instead", true
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return "", false
	}
	path := obj.Pkg().Path() + "." + obj.Name()
	if frameworkOwned[path] {
		return "edf registers the framework's own value types itself, so this answers \"unable to register a type of Ergo Framework\"", true
	}
	return "", false
}

var frameworkOwned = map[string]bool{
	"ergo.services/ergo/gen.Atom":      true,
	"ergo.services/ergo/gen.PID":       true,
	"ergo.services/ergo/gen.ProcessID": true,
	"ergo.services/ergo/gen.Event":     true,
	"ergo.services/ergo/gen.Ref":       true,
	"ergo.services/ergo/gen.Alias":     true,
	"time.Time":                        true,
}

func isErrorIface(t types.Type) bool {
	errIface, ok := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	if ok == false {
		return false
	}
	if types.IsInterface(t) {
		return types.Unalias(t).String() == "error"
	}
	return types.Implements(t, errIface)
}

func reportSliceArgs(pass *analysis.Pass, m *ergomodel.Model, call *ast.CallExpr) {
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.CompositeLit)
		if ok == false {
			continue
		}
		for _, elt := range lit.Elts {
			t := pass.TypesInfo.TypeOf(elt)
			if t == nil {
				continue
			}
			if _, isPtr := types.Unalias(t).(*types.Pointer); isPtr == false {
				reportRefused(pass, m, elt)
				continue
			}
			m.Report(pass, elt.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
					ID: ergomodel.TypeID(t), Witness: "pointer registration"},
				"a pointer type is not supported by registration; register the value instead of %s",
				exprString(pass, elt))
		}
	}
}

func placementOf(pass *analysis.Pass, callbacks map[*ast.FuncDecl]bool, fn *ast.FuncDecl) placement {
	if fn.Recv == nil && fn.Name.Name == "init" {
		return placedEarly
	}
	if fn.Name.Name == "Load" {
		return placedEarly
	}
	if callbacks[fn] {
		return placedLate
	}
	if fn.Recv == nil && fn.Name.Name == "main" && pass.Pkg.Name() == "main" {
		return placedMain
	}
	return placedHelper
}

func checkMarshalerPairs(pass *analysis.Pass, m *ergomodel.Model) {
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if ok == false {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if ok == false {
			continue
		}

		if types.IsInterface(named.Underlying()) {
			continue
		}

		marked, _ := m.MessageMarked(tn)
		if marked == false && m.Registered(named) == false {
			continue
		}
		ptr := types.NewPointer(named)

		for _, pair := range []struct{ marshal, unmarshal string }{
			{"MarshalEDF", "UnmarshalEDF"},
			{"MarshalBinary", "UnmarshalBinary"},
		} {
			hasMarshal := methodIn(named, pair.marshal) || methodIn(ptr, pair.marshal)

			valueUnmarshal := methodIn(named, pair.unmarshal)
			ptrUnmarshal := methodIn(ptr, pair.unmarshal)

			switch {
			case valueUnmarshal:
				m.Report(pass, tn.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
						ID: ergomodel.TypeID(named), Witness: "value receiver " + pair.unmarshal},
					"%s.%s has a value receiver, which registration rejects: it must be a method of *%s",
					name, pair.unmarshal, name)
			case hasMarshal && ptrUnmarshal == false:
				m.Report(pass, tn.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
						ID: ergomodel.TypeID(named), Witness: "half pair " + pair.marshal},
					"%s implements %s without a pointer receiver %s, so registration rejects the pair",
					name, pair.marshal, pair.unmarshal)
			}
		}
	}
}

func methodIn(t types.Type, name string) bool {
	ms := types.NewMethodSet(t)
	for i := 0; i < ms.Len(); i++ {
		if ms.At(i).Obj().Name() == name {
			return true
		}
	}
	return false
}

func describe(fn *ast.FuncDecl) string {
	if fn.Recv != nil {
		return "method " + fn.Name.Name
	}
	return "function " + fn.Name.Name
}

func exprString(pass *analysis.Pass, e ast.Expr) string {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return "this value"
	}
	return types.TypeString(types.Unalias(t), func(p *types.Package) string { return p.Name() })
}

func callFinding(m *ergomodel.Model, decl *ast.FuncDecl, check string) ergomodel.Finding {
	return ergomodel.Finding{
		Rule: ruleID, Kind: ergomodel.KindRegistration, Tier: 2,
		ID: m.DeclID(decl) + ":" + check, Witness: check,
	}
}

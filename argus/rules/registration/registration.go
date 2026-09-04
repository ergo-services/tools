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
  - The package level edf entry points write into a table that belongs to no node.
    Use Network().RegisterType, RegisterError and RegisterAtom, so the registration
    is owned by the node that has to talk with it.
  - An application group member with Options.InitTimeout above 15 seconds. The
    framework does not clamp it: it refuses to start the whole application.

Source: edf.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2009",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

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

func checkCalls(pass *analysis.Pass, m *ergomodel.Model) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok == false || fn.Body == nil {
				continue
			}
			placement := placementOK(m, fn)

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok == false {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok == false {
					return true
				}
				callee, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
				if ok == false || callee.Pkg() == nil {
					return true
				}
				pkg := callee.Pkg().Path()
				if strings.HasPrefix(pkg, "ergo.services/ergo/") == false {
					return true
				}

				switch callee.Name() {
				case "RegisterType":
					reportPointerArgs(pass, m, call, 0)
					reportRefusedArgs(pass, m, call, 0)
					if placement == false {
						m.Report(pass, call.Pos(),
							callFinding(m, fn, "RegisterType/placement"),
							"RegisterType in %s runs after a peer may already have connected; register in an application Load, in ApplicationSpec.Network or in init",
							describe(fn))
					}
				case "RegisterTypes":
					reportSliceArgs(pass, m, call)
					if placement == false {
						m.Report(pass, call.Pos(),
							callFinding(m, fn, "RegisterTypes/placement"),
							"RegisterTypes in %s runs after a peer may already have connected; register in an application Load, in ApplicationSpec.Network or in init",
							describe(fn))
					}
				case "RegisterTypeOf":
					if pkg == "ergo.services/ergo/net/edf" {
						reportPointerArgs(pass, m, call, 0)
						reportRefusedArgs(pass, m, call, 0)
						m.Report(pass, call.Pos(),
							callFinding(m, fn, "RegisterTypeOf"),
							"edf.RegisterTypeOf registers into the package level table; use Network().RegisterType so the registration belongs to the node")
					}
				case "RegisterTypesOf", "RegisterError", "RegisterAtom":
					if pkg == "ergo.services/ergo/net/edf" {
						if callee.Name() == "RegisterTypesOf" {
							reportSliceArgs(pass, m, call)
						}
						m.Report(pass, call.Pos(),
							callFinding(m, fn, "edf/"+callee.Name()),
							"edf.%s registers into the package level table; use Network().%s so the registration belongs to the node",
							callee.Name(), strings.TrimSuffix(callee.Name(), "Of"))
					}
				}
				return true
			})
		}
	}
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

func placementOK(m *ergomodel.Model, fn *ast.FuncDecl) bool {
	if fn.Recv == nil && fn.Name.Name == "init" {
		return true
	}
	if fn.Name.Name == "Load" {
		return true
	}

	if fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "register") {
		return true
	}
	return false
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

package alwaysfails

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2026"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2026",
	Doc: `A2026: a call the runtime rejects on its arguments, whatever the state.

A2001 reports a call that is wrong because of where it is. This one reports a call
that is wrong wherever it is: the runtime checks the argument first and returns
before doing anything, so the line has no effect at any point in the lifecycle.
Most call sites discard the error, so the code reads as if it worked.

Reported, each read straight off the runtime's own guard:

  - SendExit or SendExitAfter addressed to this process, its parent or its leader.
    The runtime refuses all three and writes a warning, because an exit signal to
    the parent would tear down the branch that owns this process.
  - SendExit, SendExitAfter, SendExitMeta or SendExitMetaAfter with a nil reason,
    which answers ErrIncorrect: a termination reason is what the receiver
    terminates with, and nil is not one.
  - SetCompressionThreshold below the framework floor of 1024, which answers
    ErrIncorrect and leaves the previous threshold in place. Note the asymmetry
    with a spawn literal, which applies a small threshold verbatim: that one is
    A3007 and is a performance note rather than a rejection.
  - SetSendPriority or SetCompressionLevel with a constant outside the declared
    enum, which answers ErrIncorrect.
  - Link, Unlink, Monitor or Demonitor addressed to this process's own PID, which
    the runtime refuses.
  - A target argument whose static type is none of gen.PID, gen.ProcessID,
    gen.Alias or gen.Atom. The parameter is any, so a plain Go string compiles and
    then answers ErrUnsupported at run time; a registered name has to be a gen.Atom.

Only intrinsic forms are reported: the target expression has to be this handle's
own accessor, and a value has to be a constant. Nothing here needs reachability or
a notion of "the current actor", which is what makes it sound.`,
	URL:      "https://docs.ergo.services/tools/argus#A2026",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

const ergoPrefix = "ergo.services/ergo/"

var targetMethods = map[string]bool{
	"Send": true, "SendImportant": true, "SendWithPriority": true,
	"SendAfter": true, "SendEvery": true,
	"SendWithPriorityAfter": true, "SendWithPriorityEvery": true,
	"Call": true, "CallWithTimeout": true, "CallWithPriority": true,
	"CallImportant": true,
	"Link":          true, "Unlink": true, "Monitor": true, "Demonitor": true,
}

var exitMethods = map[string]int{
	"SendExit": 1, "SendExitAfter": 1, "SendExitMeta": 1, "SendExitMetaAfter": 1,
}

var selfExitMethods = map[string]bool{"SendExit": true, "SendExitAfter": true}

var selfTargetMethods = map[string]bool{
	"Link": true, "Unlink": true, "Monitor": true, "Demonitor": true,
	"LinkPID": true, "UnlinkPID": true, "MonitorPID": true, "DemonitorPID": true,
}

var acceptedTargets = map[string]bool{
	"ergo.services/ergo/gen.PID":       true,
	"ergo.services/ergo/gen.ProcessID": true,
	"ergo.services/ergo/gen.Alias":     true,
	"ergo.services/ergo/gen.Atom":      true,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if ok == false || decl.Body == nil {
				return true
			}
			check(pass, m, decl)
			return true
		})
	}
	return nil, nil
}

func check(pass *analysis.Pass, m *ergomodel.Model, decl *ast.FuncDecl) {
	id := m.DeclID(decl)

	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if isSel == false {
			return true
		}
		fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
		if isFunc == false || fn.Pkg() == nil {
			return true
		}
		if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
			return true
		}
		name := fn.Name()
		handle := handlePath(sel.X)

		if idx, isExit := exitMethods[name]; isExit && idx < len(call.Args) {
			if isNil(call.Args[idx]) {
				m.Report(pass, call.Args[idx].Pos(), finding(id, name+"/reason"),
					"%s with a nil reason answers gen.ErrIncorrect: the reason is what the target terminates with, so this line does nothing",
					name)
			}
		}

		if selfExitMethods[name] && len(call.Args) > 0 && handle != "" {
			if which, isSelf := ownIdentity(pass, call.Args[0], handle); isSelf {
				m.Report(pass, call.Args[0].Pos(), finding(id, name+"/self"),
					"%s to %s always answers gen.ErrNotAllowed and logs a warning: the runtime refuses an exit signal to this process, its parent and its leader",
					name, which)
			}
		}

		if selfTargetMethods[name] && len(call.Args) > 0 && handle != "" {
			if which, isSelf := ownPID(pass, call.Args[0], handle); isSelf {
				m.Report(pass, call.Args[0].Pos(), finding(id, name+"/self"),
					"%s to %s always answers gen.ErrNotAllowed: a process cannot link to or monitor itself",
					name, which)
			}
		}

		if targetMethods[name] && len(call.Args) > 0 {
			if what, bad := unsupportedTarget(pass, call.Args[0]); bad {
				m.Report(pass, call.Args[0].Pos(), finding(id, name+"/target"),
					"%s takes the address as any, and %s is not one of gen.PID, gen.ProcessID, gen.Alias or gen.Atom, so this always answers gen.ErrUnsupported; a registered name is a gen.Atom, not a string",
					name, what)
			}
		}

		if name == "SetCompressionThreshold" && len(call.Args) > 0 {
			if v, ok := constInt(pass, call.Args[0]); ok && v < 1024 {
				m.Report(pass, call.Args[0].Pos(), finding(id, name),
					"a threshold of %d is below the framework floor of 1024, so SetCompressionThreshold answers gen.ErrIncorrect and the previous threshold stays in place",
					v)
			}
		}

		if limit, isEnum := enumMethods[name]; isEnum && len(call.Args) > 0 {
			if v, ok := constInt(pass, call.Args[0]); ok && (v < 0 || v > limit) {
				m.Report(pass, call.Args[0].Pos(), finding(id, name),
					"%d is outside the range %s accepts, so it answers gen.ErrIncorrect and the setting is unchanged",
					v, name)
			}
		}
		return true
	})
}

var enumMethods = map[string]int64{
	"SetSendPriority":     2,
	"SetCompressionLevel": 2,
}

func finding(id, check string) ergomodel.Finding {
	return ergomodel.Finding{
		Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
		ID: id + ":" + check, Witness: check,
	}
}

func ownIdentity(pass *analysis.Pass, e ast.Expr, handle string) (string, bool) {
	for _, accessor := range []string{"PID", "Parent", "Leader"} {
		if isAccessor(pass, e, handle, accessor) {
			return handle + "." + accessor + "()", true
		}
	}
	return "", false
}

func ownPID(pass *analysis.Pass, e ast.Expr, handle string) (string, bool) {
	if isAccessor(pass, e, handle, "PID") {
		return handle + ".PID()", true
	}
	return "", false
}

func isAccessor(pass *analysis.Pass, e ast.Expr, handle, method string) bool {
	call, ok := e.(*ast.CallExpr)
	if ok == false {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false || sel.Sel.Name != method {
		return false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if isFunc == false || fn.Pkg() == nil {
		return false
	}
	if strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return false
	}
	return handlePath(sel.X) == handle
}

func unsupportedTarget(pass *analysis.Pass, e ast.Expr) (string, bool) {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return "", false
	}
	u := types.Unalias(t)
	if types.IsInterface(u) {
		return "", false
	}
	if _, isParam := u.(*types.TypeParam); isParam {
		return "", false
	}
	if named, isNamed := u.(*types.Named); isNamed {
		obj := named.Obj()
		if obj == nil || obj.Pkg() == nil {
			return "", false
		}
		path := obj.Pkg().Path() + "." + obj.Name()
		if acceptedTargets[path] {
			return "", false
		}
		return obj.Pkg().Name() + "." + obj.Name(), true
	}
	basic, isBasic := u.(*types.Basic)
	if isBasic == false {
		return "", false
	}
	if basic.Kind() == types.Invalid || basic.Kind() == types.UntypedNil {
		return "", false
	}
	return "a " + basic.Name(), true
}

func constInt(pass *analysis.Pass, e ast.Expr) (int64, bool) {
	tv, ok := pass.TypesInfo.Types[e]
	if ok == false || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	return constant.Int64Val(tv.Value)
}

func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
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

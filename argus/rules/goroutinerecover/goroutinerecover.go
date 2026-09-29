package goroutinerecover

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A1010"

var Analyzer = &analysis.Analyzer{
	Name: "argusA1010",
	Doc: `A1010: a goroutine started without a panic boundary.

The framework installs a recover at every goroutine boundary it owns: the actor run
loop, meta Start and the meta message handler, application callbacks, cron jobs, the
network accept loop and the pool, router, supervisor and web worker callbacks. Each
turns a panic into termination with TerminateReasonPanic, which the supervisor sees
and acts on. That is what makes the supervision tree mean anything.

A goroutine started by application code has no such boundary. recover works only in
the goroutine whose deferred function calls it and is never inherited from the
spawner, so a bare "go func()" converts "one actor dies and restarts" into "the node
dies with every actor on it, and supervision never gets a say".

Scope decides whether the rule applies, and it is cheaper than proving panic freedom:
a goroutine reachable from an actor callback, a meta, an application or a cron job is
tier 1, one in main or a bootstrap path is not reported because failing to start is
arguably correct there, and one in a test file is exempt because a failing test is
the point.

The only accepted suppression is a boundary in the goroutine body itself: a deferred
function that calls recover, or a deferred call to a function that does. A
conditionally deferred recover counts, because every boundary in the ecosystem is
written "if lib.Recover() { defer func(){ recover() }() }".

A go statement is not the only way to start a goroutine. A call that runs a function
argument on a goroutine of its own - time.AfterFunc, sync.WaitGroup.Go, errgroup.Group.Go
and TryGo, whatever the forks surface configures, and any function of the project that
does "go f()" with its own parameter - is checked on that argument: a func literal or a
named function has to install the boundary itself. An argument this package cannot
resolve to a body, a func typed variable or field, is reported at tier 2, because whether
it recovers is unknown rather than known to be false.

A helper doing "go f()" with its own parameter is not reported at that statement. What
runs there is whatever each caller passes, so the verdict belongs at each call site, and
the helper carries it to callers in other packages as a fact.

Reach crosses packages. Whether a function starts a goroutine with no boundary, directly
or through its own calls, travels as a fact, so a callback calling into another package
of the project is reported at the call, naming the function where the goroutine starts.
The fact is recorded only for code the project can edit: a background goroutine inside
the standard library or a module cache dependency is not something this code can fix. A
go statement carrying //argus:allow A1010 is an accepted decision and does not travel to
its callers.

The preferred fix is a meta process, which has a boundary, an alias the parent can
link or monitor, and a documented lifecycle. The fallback is to hoist the process
handle, PID and logger into locals, reference only those in the recover body, and
send a message back to the actor.

A goroutine that also touches actor state is a separate finding (A1004) on the same
construct: a race free goroutine can still take the node down, so two deliberate
exceptions on one go statement take two rule ids.`,
	URL:      "https://docs.ergo.services/tools/argus#A1010",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.GoSites() {
		if site.Recovered || site.Forwarded {
			continue
		}
		if inTest(pass, site.Stmt.Pos()) {
			continue
		}
		where, id, ok := scope(m, site.In, site.Enclosing)
		if ok == false {
			continue
		}
		m.Report(pass, site.Stmt.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1, ID: id},
			"goroutine started in %s has no recover, so a panic in it takes down the whole node instead of this one process; defer a recover in the goroutine body or move the work into a meta process",
			where)
	}

	for _, site := range m.ForkSites() {
		if site.Recovered {
			continue
		}
		if inTest(pass, site.Call.Pos()) {
			continue
		}
		where, id, ok := scope(m, site.In, site.Enclosing)
		if ok == false {
			continue
		}
		if site.Resolved == false {
			m.Report(pass, site.Arg.Pos(),
				ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
					ID: id + ":" + site.Via, Witness: "unresolved func value"},
				"%s hands %s to %s, which runs it on a goroutine of its own, and what it points to does not resolve here, so nothing shows a recover in it; a panic there would take down the whole node instead of this one process. Pass a func literal or a named function that defers a recover",
				where, site.What, site.Via)
			continue
		}
		m.Report(pass, site.Arg.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
				ID: id + ":" + site.Via, Witness: site.What},
			"%s hands %s to %s, which runs it on a goroutine of its own, and %s defers no recover, so a panic in it takes down the whole node instead of this one process; defer a recover at the top of %s or move the work into a meta process",
			where, site.What, site.Via, site.What, site.What)
	}

	byDecl := map[*ast.FuncDecl]*ergomodel.Callback{}
	for _, cb := range m.Callbacks {
		byDecl[cb.Decl] = cb
	}
	for _, file := range pass.Files {
		for _, d := range file.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if ok == false || decl.Body == nil {
				continue
			}
			if inTest(pass, decl.Pos()) {
				continue
			}
			where, id, ok := scope(m, byDecl[decl], m.EnclosingFunc(decl))
			if ok == false {
				continue
			}
			crossPackage(pass, m, decl, where, id)
		}
	}
	return nil, nil
}

func crossPackage(pass *analysis.Pass, m *ergomodel.Model, decl *ast.FuncDecl, where, id string) {
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isGo := n.(*ast.GoStmt); isGo {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if ok == false {
			return true
		}
		callee := calleeFunc(pass, call)
		if callee == nil || callee.Pkg() == nil || callee.Pkg() == pass.Pkg {
			return true
		}
		b := m.Behavior(callee)
		if b.Unrecovered == false {
			return true
		}
		m.Report(pass, call.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1,
				ID: id + ":" + ergomodel.FuncID(callee), Witness: b.UnrecoveredIn},
			"%s calls %s, which %s a goroutine with no recover (%s), so a panic in it takes down the whole node instead of this one process; defer a recover in that goroutine body or move the work into a meta process",
			where, display(callee), reaches(callee, b.UnrecoveredIn), b.UnrecoveredAt)
		return true
	})
}

func reaches(callee *types.Func, in string) string {
	if in == "" || in == ergomodel.FuncID(callee) {
		return "starts"
	}
	return "reaches, through " + in + ","
}

func scope(m *ergomodel.Model, in *ergomodel.Callback, enclosing *types.Func) (string, string, bool) {
	if in != nil {
		return in.Name, ergomodel.CallbackID(in), true
	}
	if m.UnderCallback(enclosing) {
		return "a path reachable from a callback", ergomodel.FuncID(enclosing), true
	}
	return "", "", false
}

func inTest(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go")
}

func display(fn *types.Func) string {
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		t := sig.Recv().Type()
		if p, isPtr := types.Unalias(t).(*types.Pointer); isPtr {
			t = p.Elem()
		}
		if named, isNamed := types.Unalias(t).(*types.Named); isNamed {
			return named.Obj().Name() + "." + fn.Name()
		}
	}
	return fn.Pkg().Name() + "." + fn.Name()
}

func calleeFunc(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[fun].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	}
	return nil
}

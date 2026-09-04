package goroutinerecover

import (
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
		if site.Recovered {
			continue
		}

		if strings.HasSuffix(pass.Fset.Position(site.Stmt.Pos()).Filename, "_test.go") {
			continue
		}

		if site.In == nil && m.UnderCallback(site.Enclosing) == false {
			continue
		}

		where := "a path reachable from a callback"
		id := ergomodel.FuncID(site.Enclosing)
		if site.In != nil {
			where = site.In.Name
			id = ergomodel.CallbackID(site.In)
		}
		m.Report(pass, site.Stmt.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 1, ID: id},
			"goroutine started in %s has no recover, so a panic in it takes down the whole node instead of this one process; defer a recover in the goroutine body or move the work into a meta process",
			where)
	}
	return nil, nil
}

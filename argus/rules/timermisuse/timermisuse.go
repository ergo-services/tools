package timermisuse

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2006"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2006",
	Doc: `A2006: timer misuse in an actor.

Two defects. A non-positive period on SendEvery or SendWithPriorityEvery is rejected
with ErrIncorrect, so the ticker never arms and the work silently never happens.

And a raw time.NewTicker, time.Tick or time.After used to delay or repeat work inside
a callback bypasses the mailbox: the tick arrives on the timer goroutine rather than
as a message, so it is not ordered against the actor's other messages, not visible to
inspection, and not stopped when the process terminates. Use SendAfter or SendEvery,
which deliver a real message to the actor.

A time.After in a select arm is bounding a wait rather than scheduling work, which is
the documented way to bound a channel receive, and is not reported.

Source: messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2006",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

var periodArg = map[string]int{
	"SendEvery":             2,
	"SendWithPriorityEvery": 3,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	for _, site := range m.SendSites {
		idx, ok := periodArg[site.Method]
		if ok == false || idx >= len(site.Call.Args) {
			continue
		}
		arg := site.Call.Args[idx]
		tv, ok := pass.TypesInfo.Types[arg]
		if ok == false || tv.Value == nil {
			continue
		}
		if positive(pass, arg) {
			continue
		}
		m.Report(pass, arg.Pos(),
			ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(site.In) + ":" + site.Method, Witness: "non-positive period"},
			"%s with a period of zero or less returns ErrIncorrect, so the ticker never arms",
			site.Method)
	}

	for _, cb := range m.Callbacks {
		if cb.Kind == ergomodel.CBMetaStart && cb.Meta {
			continue
		}
		inSelect := 0
		var walk func(n ast.Node) bool
		walk = func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectStmt:
				inSelect++
				for _, stmt := range x.Body.List {
					ast.Inspect(stmt, walk)
				}
				inSelect--
				return false
			case *ast.CallExpr:
				name, ok := timeScheduler(pass.TypesInfo, x)
				if ok == false {
					return true
				}

				if name == "After" && inSelect > 0 {
					return true
				}
				m.Report(pass, x.Pos(),
					ergomodel.Finding{Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
						ID: ergomodel.CallbackID(cb) + ":time." + name, Witness: "raw timer"},
					"time.%s inside %s schedules work off the mailbox; use SendAfter or SendEvery so the tick arrives as a message",
					name, cb.Name)
			}
			return true
		}
		ast.Inspect(cb.Decl.Body, walk)
	}
	return nil, nil
}

func positive(pass *analysis.Pass, e ast.Expr) bool {
	tv := pass.TypesInfo.Types[e]
	if tv.Value == nil {
		return true
	}
	s := tv.Value.String()
	if strings.HasPrefix(s, "-") {
		return false
	}
	return s != "0"
}

func timeScheduler(info *types.Info, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if ok == false {
		return "", false
	}
	switch sel.Sel.Name {
	case "NewTicker", "Tick", "After", "NewTimer", "AfterFunc":
	default:
		return "", false
	}
	obj := info.Uses[sel.Sel]
	if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != "time" {
		return "", false
	}
	return sel.Sel.Name, true
}

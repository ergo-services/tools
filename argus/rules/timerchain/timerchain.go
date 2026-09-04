package timerchain

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"ergo.tools/argus/ergomodel"
)

const ruleID = "A2015"

var Analyzer = &analysis.Analyzer{
	Name: "argusA2015",
	Doc: `A2015: a self timer chain armed from more than one place.

A SendAfter to self whose handler arms the next one is the framework's ticker: one
message in flight, one timer alive, indefinitely. It works because each delivery arms
exactly one successor. Arm the same message from a second place and there are now two
independent chains, each self sustaining, and nothing merges them: the handler cannot
tell a tick of one chain from a tick of the other. Every extra arming doubles the rate
permanently, and if the handler also makes a request, the mailbox is running that many
concurrent blocking waits.

Two forms are reported, both requiring the cancel to be discarded, because a kept
cancel is the mechanism for stopping the previous chain before starting another.

Two armings of the same message as separate statements of one list inside its own
handler. Both run on every delivery, so the chain doubles on each tick.

An arming inside the handler for a different message, while the message's own handler
also arms it. The chain already exists, and every arrival of the unrelated message
starts a parallel one. This is what a retry path looks like: the error case re-arms
the tick to try again sooner, and the original chain is still running.

A discarded cancel from SendEvery is A2006's, not this rule's, and a single arming in
Init plus a re-arm in the handler is the correct idiom and stays silent. Two armings in
two arms of one branch are also silent, since exactly one of them runs.

The fix is to keep the cancel and stop the previous chain before arming a new one, or
to route the retry through the existing chain instead of starting one.

Source: messages.md`,
	URL:      "https://docs.ergo.services/tools/argus#A2015",
	Requires: []*analysis.Analyzer{ergomodel.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	m := ergomodel.From(pass)

	byBehavior := map[*types.Named][]arming{}
	for _, cb := range m.Callbacks {
		for _, a := range armings(pass, m, cb) {
			byBehavior[cb.Behavior] = append(byBehavior[cb.Behavior], a)
		}
	}

	for _, list := range byBehavior {
		for _, group := range byMessage(list) {
			reportGroup(pass, m, group)
		}
	}
	return nil, nil
}

type arming struct {
	pos      token.Pos
	message  string
	short    string
	cb       *ergomodel.Callback
	stmt     ast.Stmt
	list     *[]ast.Stmt
	caseType string
}

func reportGroup(pass *analysis.Pass, m *ergomodel.Model, group []arming) {
	if len(group) < 2 {
		return
	}

	owner := -1
	for i, a := range group {
		if a.caseType == a.message {
			owner = i
			break
		}
	}
	if owner < 0 {
		return
	}

	for i, a := range group {
		if i == owner {
			continue
		}
		witness := ""
		switch {
		case a.caseType == a.message && a.list == group[owner].list:
			witness = "armed twice in one list"
		case a.caseType != "" && a.caseType != a.message:
			witness = "armed from " + short(a.caseType)
		default:
			continue
		}
		m.Report(pass, a.pos,
			ergomodel.Finding{
				Rule: ruleID, Kind: ergomodel.KindCallback, Tier: 2,
				ID: ergomodel.CallbackID(a.cb), Witness: witness,
			},
			"this arms a %s chain that %s already re-arms on every delivery, and the cancel is discarded, so the two chains run in parallel forever and every arrival here adds another; keep the cancel and stop the previous chain first, or route this through the existing chain",
			a.short, group[owner].cb.Name)
	}
}

func byMessage(list []arming) [][]arming {
	seen := map[string][]arming{}
	for _, a := range list {
		seen[a.message] = append(seen[a.message], a)
	}
	var out [][]arming
	for _, group := range seen {
		out = append(out, group)
	}
	return out
}

func armings(pass *analysis.Pass, m *ergomodel.Model, cb *ergomodel.Callback) []arming {
	if cb.Decl.Body == nil {
		return nil
	}
	var out []arming

	var walk func(list *[]ast.Stmt, caseType string)
	walk = func(list *[]ast.Stmt, caseType string) {
		for i := range *list {
			stmt := (*list)[i]
			if a, ok := armingIn(pass, m, cb, stmt, list, caseType); ok {
				out = append(out, a...)
			}
			switch x := stmt.(type) {
			case *ast.TypeSwitchStmt:
				for _, clause := range x.Body.List {
					cc, isCase := clause.(*ast.CaseClause)
					if isCase == false {
						continue
					}
					inner := caseType
					if len(cc.List) == 1 {
						if t := typePathOf(pass, cc.List[0]); t != "" {
							inner = t
						}
					}
					walk(&cc.Body, inner)
				}
			case *ast.SwitchStmt:
				for _, clause := range x.Body.List {
					if cc, isCase := clause.(*ast.CaseClause); isCase {
						walk(&cc.Body, caseType)
					}
				}
			case *ast.SelectStmt:
				for _, clause := range x.Body.List {
					if cc, isComm := clause.(*ast.CommClause); isComm {
						walk(&cc.Body, caseType)
					}
				}
			case *ast.IfStmt:
				walk(&x.Body.List, caseType)
				if block, isBlock := x.Else.(*ast.BlockStmt); isBlock {
					walk(&block.List, caseType)
				}
			case *ast.ForStmt:
				walk(&x.Body.List, caseType)
			case *ast.RangeStmt:
				walk(&x.Body.List, caseType)
			case *ast.BlockStmt:
				walk(&x.List, caseType)
			}
		}
	}
	walk(&cb.Decl.Body.List, "")
	return out
}

func armingIn(pass *analysis.Pass, m *ergomodel.Model, cb *ergomodel.Callback,
	stmt ast.Stmt, list *[]ast.Stmt, caseType string) ([]arming, bool) {

	call, discarded := timerCall(stmt)
	if call == nil || discarded == false {
		return nil, false
	}
	fn := calleeFunc(pass, call)
	if fn == nil || fn.Name() != "SendAfter" || fn.Pkg() == nil {
		return nil, false
	}
	if strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/") == false {
		return nil, false
	}
	if len(call.Args) < 2 || selfTarget(pass, m, cb, call.Args[0]) == false {
		return nil, false
	}
	path := typePathOf(pass, call.Args[1])
	if path == "" {
		return nil, false
	}
	return []arming{{
		pos: call.Pos(), message: path, short: short(path),
		cb: cb, stmt: stmt, list: list, caseType: caseType,
	}}, true
}

func timerCall(stmt ast.Stmt) (*ast.CallExpr, bool) {
	switch x := stmt.(type) {
	case *ast.ExprStmt:
		if call, ok := x.X.(*ast.CallExpr); ok {
			return call, true
		}
	case *ast.AssignStmt:
		if len(x.Rhs) != 1 {
			return nil, false
		}
		call, ok := x.Rhs[0].(*ast.CallExpr)
		if ok == false {
			return nil, false
		}
		id, isIdent := x.Lhs[0].(*ast.Ident)
		return call, isIdent && id.Name == "_"
	}
	return nil, false
}

func selfTarget(pass *analysis.Pass, m *ergomodel.Model, cb *ergomodel.Callback, e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if ok == false {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if isSel == false {
		return false
	}
	recv, isIdent := sel.X.(*ast.Ident)
	if isIdent == false || recv.Name != cb.Recv {
		return false
	}
	switch sel.Sel.Name {
	case "PID", "Name":
		fn := calleeFunc(pass, call)
		return fn != nil && fn.Pkg() != nil &&
			strings.HasPrefix(fn.Pkg().Path(), "ergo.services/ergo/")
	}
	return false
}

func typePathOf(pass *analysis.Pass, e ast.Expr) string {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return ""
	}
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := types.Unalias(t).(*types.Named)
	if ok == false || named.Obj() == nil || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

func short(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
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

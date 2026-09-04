package ergomodel

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
)

type Kind string

const (
	KindShape        Kind = "shape"
	KindCallback     Kind = "callback"
	KindEscape       Kind = "escape"
	KindSpec         Kind = "spec"
	KindEvent        Kind = "event"
	KindRegistration Kind = "registration"
	KindDirective    Kind = "directive"
)

type Finding struct {
	Rule string
	Kind Kind
	ID   string
	Tier int

	Witness string
}

type suppression string

const (
	suppressedByDirective suppression = "directive"
	suppressedByBaseline  suppression = "baseline"
	suppressedByPackage   suppression = "package allowlist"
)

type debtCounter struct {
	mu         sync.Mutex
	suppressed map[string]map[suppression]int
	seen       map[string]int
}

func newDebtCounter() *debtCounter {
	return &debtCounter{
		suppressed: map[string]map[suppression]int{},
		seen:       map[string]int{},
	}
}

func (d *debtCounter) note(rule string, why suppression) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.suppressed[rule] == nil {
		d.suppressed[rule] = map[suppression]int{}
	}
	d.suppressed[rule][why]++
}

func (d *debtCounter) occurrence(key string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[key]++
	return d.seen[key]
}

func (d *debtCounter) snapshot() map[string]map[suppression]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]map[suppression]int, len(d.suppressed))
	for rule, reasons := range d.suppressed {
		copied := make(map[suppression]int, len(reasons))
		for why, n := range reasons {
			copied[why] = n
		}
		out[rule] = copied
	}
	return out
}

func (m *Model) DebtLine() (string, bool) {
	counts := m.debt2.snapshot()
	if len(counts) == 0 {
		return "", false
	}
	rules := make([]string, 0, len(counts))
	for rule := range counts {
		rules = append(rules, rule)
	}
	sort.Strings(rules)

	var parts []string
	for _, rule := range rules {
		reasons := counts[rule]
		total := 0
		for _, n := range reasons {
			total += n
		}
		var detail []string
		for _, why := range []suppression{suppressedByDirective, suppressedByBaseline, suppressedByPackage} {
			if n := reasons[why]; n > 0 {
				detail = append(detail, fmt.Sprintf("%d by %s", n, why))
			}
		}
		parts = append(parts, fmt.Sprintf("%s %d (%s, tier severity %s)",
			rule, total, strings.Join(detail, ", "), m.severityName(rule)))
	}
	return strings.Join(parts, "; "), true
}

func (m *Model) severityName(rule string) string {
	tier := 0
	if len(rule) > 1 {
		tier = int(rule[1] - '0')
	}
	switch m.cfg.Severity(tier) {
	case SeverityError:
		return "error"
	case SeverityWarn:
		return "warn"
	}
	return "off"
}

func (m *Model) Report(pass *analysis.Pass, pos token.Pos, f Finding, format string, args ...any) {
	m.ReportFix(pass, pos, pos, f, nil, format, args...)
}

func (m *Model) ReportFix(pass *analysis.Pass, pos, end token.Pos, f Finding,
	fixes []analysis.SuggestedFix, format string, args ...any) {

	debtRule := f.Rule == "A3005"

	if m.pkgAllowed && debtRule == false {
		m.debt2.note(f.Rule, suppressedByPackage)
		return
	}

	if m.editable() == false {
		return
	}
	if m.cfg.Severity(f.Tier) == SeverityOff {
		return
	}

	if m.cfg.Tests == false && strings.HasSuffix(m.fset.Position(pos).Filename, "_test.go") {
		return
	}
	if debtRule == false && m.Suppressed(pos, f.Rule) {
		m.debt2.note(f.Rule, suppressedByDirective)
		return
	}

	body := fmt.Sprintf(format, args...)

	if debtRule == false {
		accepted, stale := m.acceptFinding(f)
		if stale != "" {

			m.reportStaleBaseline(pass, pos, f, stale)
		}
		if accepted {
			m.debt2.note(f.Rule, suppressedByBaseline)
			return
		}
	}

	msg := fmt.Sprintf("[tier%d] [%s] %s", f.Tier, f.Rule, body)
	if flagKeys {
		msg += fmt.Sprintf(" [argus-key: %s %s %s]", f.Rule, f.Kind, f.ID)
	}

	if debtRule == false {
		fixes = append(fixes, m.directiveFix(pass, pos, f.Rule))
	}

	d := analysis.Diagnostic{Pos: pos, End: end, Message: msg, SuggestedFixes: fixes}
	if m.cfg.Severity(f.Tier) == SeverityError {
		pass.Report(d)
		return
	}

	fmt.Fprintf(os.Stderr, "%s: %s\n", m.fset.Position(pos), msg)
}

func (m *Model) directiveFix(pass *analysis.Pass, pos token.Pos, rule string) analysis.SuggestedFix {
	position := m.fset.Position(pos)
	indent := ""
	if position.Column > 1 {
		indent = strings.Repeat("\t", 1)
	}
	insert := token.Pos(int(pos) - (position.Column - 1))
	return analysis.SuggestedFix{
		Message: fmt.Sprintf("record an exception for %s", rule),
		TextEdits: []analysis.TextEdit{{
			Pos:     insert,
			End:     insert,
			NewText: []byte(fmt.Sprintf("%s//argus:allow %s <reason>\n", indent, rule)),
		}},
	}
}

func (m *Model) acceptFinding(f Finding) (accepted bool, staleWitness string) {
	if m.baseline == nil || len(m.baseline.entries) == 0 {
		return false, ""
	}
	entry, ok := m.baseline.entries[baselineIndex(f.Rule, string(f.Kind), f.ID)]
	if ok == false {
		return false, ""
	}
	if entry.Witness != "" && f.Witness != "" && entry.Witness != f.Witness {
		staleWitness = entry.Witness
	}

	limit := entry.Count
	if limit <= 0 {
		limit = 1
	}
	return m.debt2.occurrence(baselineIndex(f.Rule, string(f.Kind), f.ID)) <= limit, staleWitness
}

func (m *Model) reportStaleBaseline(pass *analysis.Pass, pos token.Pos, f Finding, recorded string) {
	m.Report(pass, pos, Finding{
		Rule: "A3005", Kind: KindDirective, Tier: 3,
		ID: f.Rule + "/" + string(f.Kind) + "/" + f.ID,
	},
		"the baseline entry for %s %s %s records witness %q but the finding now says %q, so the accepted decision no longer describes this code",
		f.Rule, f.Kind, f.ID, recorded, f.Witness)
}

func CallbackID(cb *Callback) string {
	if cb == nil {
		return ""
	}
	return typePath(cb.Behavior) + "." + cb.Name
}

func TypeID(t types.Type) string {
	if t == nil {
		return "?"
	}
	if named, ok := deref(t).(*types.Named); ok {
		if path := typePath(named); path != "" {
			return path
		}
	}
	return types.TypeString(types.Unalias(t), nil)
}

func FuncID(fn *types.Func) string {
	if fn == nil {
		return "?"
	}
	if recv := recvTypePath(fn); recv != "" {
		return recv + "." + fn.Name()
	}
	if fn.Pkg() != nil {
		return fn.Pkg().Path() + "." + fn.Name()
	}
	return fn.Name()
}

func (m *Model) DeclID(decl *ast.FuncDecl) string {
	if decl == nil {
		return m.pass.Pkg.Path()
	}
	if fn, ok := m.pass.TypesInfo.Defs[decl.Name].(*types.Func); ok {
		return FuncID(fn)
	}
	return m.pass.Pkg.Path() + "." + decl.Name.Name
}

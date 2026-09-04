package ergomodel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineKeyIsAnIdentity(t *testing.T) {
	base := baselineIndex("A1001", "shape", "example.com/app.Order")
	if base != baselineIndex("A1001", "shape", "example.com/app.Order") {
		t.Fatal("the key must be deterministic")
	}
	for _, other := range []string{
		baselineIndex("A2004", "shape", "example.com/app.Order"),
		baselineIndex("A1001", "callback", "example.com/app.Order"),
		baselineIndex("A1001", "shape", "example.com/app.Invoice"),
	} {
		if base == other {
			t.Errorf("two different identities collided on %q", base)
		}
	}
}

func TestBaselineCountAcceptsFirstN(t *testing.T) {
	m := &Model{
		baseline: &baseline{entries: map[string]BaselineEntry{
			baselineIndex("A1001", "shape", "app.Order"): {
				Rule: "A1001", Kind: "shape", ID: "app.Order", Count: 2,
			},
			baselineIndex("A1001", "shape", "app.Invoice"): {
				Rule: "A1001", Kind: "shape", ID: "app.Invoice",
			},
		}},
		debt2: newDebtCounter(),
	}

	order := Finding{Rule: "A1001", Kind: KindShape, ID: "app.Order"}
	for i := 1; i <= 2; i++ {
		if accepted, _ := m.acceptFinding(order); accepted == false {
			t.Errorf("occurrence %d of a count-2 entry must be accepted", i)
		}
	}
	if accepted, _ := m.acceptFinding(order); accepted {
		t.Error("the third occurrence of a count-2 entry must report: a defect that spread is new")
	}

	invoice := Finding{Rule: "A1001", Kind: KindShape, ID: "app.Invoice"}
	if accepted, _ := m.acceptFinding(invoice); accepted == false {
		t.Error("the first occurrence of an entry with no count must be accepted")
	}
	if accepted, _ := m.acceptFinding(invoice); accepted {
		t.Error("an absent count means one, so the second occurrence must report")
	}

	if accepted, _ := m.acceptFinding(Finding{Rule: "A1001", Kind: KindShape, ID: "app.Other"}); accepted {
		t.Error("an unrecorded identity must not be accepted")
	}
}

func TestBaselineStaleWitness(t *testing.T) {
	m := &Model{
		baseline: &baseline{entries: map[string]BaselineEntry{
			baselineIndex("A1001", "shape", "app.Order"): {
				Rule: "A1001", Kind: "shape", ID: "app.Order", Count: 5,
				Witness: "Items []string",
			},
		}},
		debt2: newDebtCounter(),
	}

	same := Finding{Rule: "A1001", Kind: KindShape, ID: "app.Order", Witness: "Items []string"}
	accepted, stale := m.acceptFinding(same)
	if accepted == false {
		t.Error("a matching witness must still be accepted")
	}
	if stale != "" {
		t.Errorf("a matching witness is not stale, got %q", stale)
	}

	changed := Finding{Rule: "A1001", Kind: KindShape, ID: "app.Order", Witness: "Cache *app.Store"}
	accepted, stale = m.acceptFinding(changed)
	if stale != "Items []string" {
		t.Errorf("stale witness = %q, want the recorded one", stale)
	}
	if accepted == false {
		t.Error("a stale witness is reported alongside acceptance, not instead of it: the lookup is by identity")
	}
}

func TestBaselineRoundTrip(t *testing.T) {
	line := "/src/worker/actor.go:41:12: [tier1] [A1001] whatever the text says " +
		"[argus-key: A1001 shape example.com/app.Order]"

	entry, position, ok := ParseKeyedDiagnostic(line)
	if ok == false {
		t.Fatalf("the emitted form must parse back: %q", line)
	}
	if position != "/src/worker/actor.go:41:12" {
		t.Errorf("position = %q, want the location prefix so one site seen twice folds", position)
	}
	if entry.Rule != "A1001" || entry.Kind != "shape" || entry.ID != "example.com/app.Order" {
		t.Errorf("parsed %+v, want the three key components", entry)
	}
	if entry.Count != 1 {
		t.Errorf("count = %d, want 1 for a single occurrence", entry.Count)
	}

	suffixed := strings.Replace(line, "A1001 shape", "A2001a callback", 1)
	if e, _, ok := ParseKeyedDiagnostic(suffixed); ok == false || e.Rule != "A2001a" {
		t.Errorf("A2001a did not round trip: %+v", e)
	}

	for _, noise := range []string{
		"",
		"# example.com/app/worker",
		"/src/worker/actor.go:41:12: [tier1] [A1001] no key here",
		"vet: typechecking failed",
		"/src/x.go:1:1: [tier1] [A1001] short key [argus-key: A1001 shape]",
	} {
		if _, _, ok := ParseKeyedDiagnostic(noise); ok {
			t.Errorf("%q must not parse as a keyed diagnostic", noise)
		}
	}
}

func TestBaselineWriteIsStable(t *testing.T) {
	entries := []BaselineEntry{
		{Rule: "A2004", Kind: "shape", ID: "b/pkg.Late"},
		{Rule: "A1001", Kind: "shape", ID: "a/pkg.Beta"},
		{Rule: "A1001", Kind: "callback", ID: "a/pkg.Worker.Terminate"},
	}
	first, err := WriteBaseline(entries)
	if err != nil {
		t.Fatal(err)
	}

	second, err := WriteBaseline([]BaselineEntry{entries[2], entries[0], entries[1]})
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("output depends on input order:\n%s\nvs\n%s", first, second)
	}

	var file BaselineFile
	if err := json.Unmarshal(first, &file); err != nil {
		t.Fatalf("output is not valid json: %s", err)
	}
	if file.Version != 1 {
		t.Errorf("version = %d, want 1", file.Version)
	}
	if len(file.Entries) != 3 {
		t.Errorf("entries = %d, want 3", len(file.Entries))
	}
}

func TestLoadBaseline(t *testing.T) {
	dir := t.TempDir()

	b, err := loadBaseline(filepath.Join(dir, "absent.json"))
	if err != nil {
		t.Fatalf("a missing baseline must not be an error: %s", err)
	}
	if len(b.entries) != 0 {
		t.Error("a missing baseline must accept nothing")
	}

	path := filepath.Join(dir, "argus-baseline.json")
	data, err := WriteBaseline([]BaselineEntry{
		{Rule: "A1001", Kind: "shape", ID: "a/pkg.Order", Count: 3, Note: "reviewed"},

		{Rule: "A9999", Kind: "shape", ID: "a/pkg.Ghost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	b, err = loadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := b.entries[baselineIndex("A1001", "shape", "a/pkg.Order")]
	if ok == false {
		t.Fatal("the written entry did not load back")
	}
	if entry.Count != 3 {
		t.Errorf("count = %d, want 3", entry.Count)
	}
	if _, ok := b.entries[baselineIndex("A9999", "shape", "a/pkg.Ghost")]; ok {
		t.Error("an entry naming an unknown rule must not enter the lookup")
	}
	unknown := UnknownRuleEntries(path)
	if len(unknown) != 1 || unknown[0].Rule != "A9999" {
		t.Errorf("UnknownRuleEntries = %+v, want the one ghost entry", unknown)
	}

	for name, body := range map[string]string{
		"bad.json":     "{not json",
		"future.json":  `{"version":99,"entries":[]}`,
		"partial.json": `{"version":1,"entries":[{"rule":"A1001","kind":"shape"}]}`,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadBaseline(p); err == nil {
			t.Errorf("%s must be a load error", name)
		}
	}
}

func TestResolveBaselinePath(t *testing.T) {
	cfg := &Config{Baseline: "argus-baseline.json", path: "/project/argus.yml"}
	if got := resolveBaselinePath(cfg); got != "/project/argus-baseline.json" {
		t.Errorf("resolved = %q, want it next to argus.yml", got)
	}
	abs := &Config{Baseline: "/elsewhere/base.json", path: "/project/argus.yml"}
	if got := resolveBaselinePath(abs); got != "/elsewhere/base.json" {
		t.Errorf("resolved = %q, want the absolute path unchanged", got)
	}
	if got := resolveBaselinePath(&Config{}); got != "" {
		t.Errorf("resolved = %q, want empty when no baseline is configured", got)
	}
}

func TestDebtLine(t *testing.T) {
	m := &Model{
		cfg:   DefaultConfig(),
		debt2: newDebtCounter(),
	}
	if _, ok := m.DebtLine(); ok {
		t.Error("a package with no suppressions has no debt line")
	}

	m.debt2.note("A1001", suppressedByDirective)
	m.debt2.note("A1001", suppressedByDirective)
	m.debt2.note("A1001", suppressedByBaseline)
	m.debt2.note("A2011", suppressedByPackage)

	line, ok := m.DebtLine()
	if ok == false {
		t.Fatal("expected a debt line")
	}
	for _, want := range []string{
		"A1001 3", "2 by directive", "1 by baseline",
		"A2011 1", "1 by package allowlist",
		"tier severity error", "tier severity warn",
	} {
		if strings.Contains(line, want) == false {
			t.Errorf("debt line %q is missing %q", line, want)
		}
	}
	fmt.Fprintln(os.Stderr, "debt line:", line)
}

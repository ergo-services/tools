package ergomodel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type BaselineEntry struct {
	Rule string `json:"rule"`
	Kind string `json:"kind"`
	ID   string `json:"id"`

	Count int `json:"count,omitempty"`

	Witness string `json:"witness,omitempty"`
	Note    string `json:"note,omitempty"`
}

type BaselineFile struct {
	Version int             `json:"version"`
	Entries []BaselineEntry `json:"entries"`
}

type baseline struct {
	entries map[string]BaselineEntry
}

var (
	baselineMu    sync.Mutex
	baselineCache = map[string]*baseline{}
)

func loadBaseline(path string) (*baseline, error) {
	if path == "" {
		return &baseline{}, nil
	}

	baselineMu.Lock()
	defer baselineMu.Unlock()

	if b, ok := baselineCache[path]; ok {
		return b, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			b := &baseline{}
			baselineCache[path] = b
			return b, nil
		}
		return nil, fmt.Errorf("argus: read baseline %q: %w", path, err)
	}

	var file BaselineFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("argus: parse baseline %q: %w", path, err)
	}

	if file.Version != 0 && file.Version != 1 {
		return nil, fmt.Errorf("argus: baseline %q has unsupported version %d", path, file.Version)
	}

	b := &baseline{entries: make(map[string]BaselineEntry, len(file.Entries))}
	for _, e := range file.Entries {
		if e.Rule == "" || e.Kind == "" || e.ID == "" {
			return nil, fmt.Errorf("argus: baseline %q has an entry missing rule, kind or id", path)
		}
		if KnownRuleIDs[e.Rule] == false {

			continue
		}
		b.entries[baselineIndex(e.Rule, e.Kind, e.ID)] = e
	}
	baselineCache[path] = b
	return b, nil
}

func baselineIndex(rule, kind, id string) string {
	return rule + "\x00" + kind + "\x00" + id
}

func UnknownRuleEntries(path string) []BaselineEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file BaselineFile
	if json.Unmarshal(data, &file) != nil {
		return nil
	}
	var out []BaselineEntry
	for _, e := range file.Entries {
		if e.Rule != "" && KnownRuleIDs[e.Rule] == false {
			out = append(out, e)
		}
	}
	return out
}

func (m *Model) BaselinePath() string { return resolveBaselinePath(m.cfg) }

func WriteBaseline(entries []BaselineEntry) ([]byte, error) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Rule != entries[j].Rule {
			return entries[i].Rule < entries[j].Rule
		}
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].ID < entries[j].ID
	})
	out, err := json.MarshalIndent(BaselineFile{Version: 1, Entries: entries}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func ParseKeyedDiagnostic(line string) (entry BaselineEntry, position string, ok bool) {

	open := strings.LastIndex(line, "[argus-key: ")
	if open < 0 || strings.HasSuffix(strings.TrimSpace(line), "]") == false {
		return BaselineEntry{}, "", false
	}
	inner := strings.TrimSuffix(strings.TrimSpace(line[open+len("[argus-key: "):]), "]")
	fields := strings.Fields(inner)
	if len(fields) != 3 {
		return BaselineEntry{}, "", false
	}
	if i := strings.Index(line, ": ["); i >= 0 {
		position = line[:i]
	}
	return BaselineEntry{Rule: fields[0], Kind: fields[1], ID: fields[2], Count: 1}, position, true
}

func resolveBaselinePath(cfg *Config) string {
	if cfg == nil || cfg.Baseline == "" {
		return ""
	}
	if filepath.IsAbs(cfg.Baseline) {
		return cfg.Baseline
	}
	if cfg.path == "" {
		abs, err := filepath.Abs(cfg.Baseline)
		if err != nil {
			return cfg.Baseline
		}
		return abs
	}
	return filepath.Join(filepath.Dir(cfg.path), cfg.Baseline)
}

package ergomodel

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type Severity int

const (
	SeverityOff Severity = iota
	SeverityWarn
	SeverityError
)

func parseSeverity(s string) (Severity, error) {
	switch strings.TrimSpace(s) {
	case "off":
		return SeverityOff, nil
	case "warn":
		return SeverityWarn, nil
	case "error":
		return SeverityError, nil
	}
	return SeverityOff, fmt.Errorf("unknown severity %q, want off, warn or error", s)
}

type BlockingSurface struct {
	Func   string
	Recv   string
	Method string
	Why    string

	Transitive bool
}

type RoundTripSurface struct {
	Func   string
	Recv   string
	Method string
	Why    string
}

type SenderSurface struct {
	Recv   string
	Method string
	Param  int
}

type CallbackSurface struct {
	Recv    string
	Methods []string
}

type Config struct {
	Tiers         map[int]Severity
	AllowTypes    map[string]string
	AllowPackages map[string]string
	AcceptIfaces  map[string]bool
	Blocking      []BlockingSurface
	RoundTrips    []RoundTripSurface
	Senders       []SenderSurface
	Callbacks     []CallbackSurface
	Baseline      string
	Tests         bool

	path string
}

func DefaultConfig() *Config {
	return &Config{
		Tiers: map[int]Severity{1: SeverityError, 2: SeverityWarn, 3: SeverityOff},
		AllowTypes: map[string]string{
			"time.Time":                             "Location is a shared immutable singleton",
			"net/netip.Addr":                        "interned representation is immutable",
			"net.Addr":                              "an address returned by net is fresh and never mutated by convention",
			"math/big.Int":                          "value semantics by convention when not mutated after send",
			"math/big.Float":                        "value semantics by convention when not mutated after send",
			"math/big.Rat":                          "value semantics by convention when not mutated after send",
			"github.com/shopspring/decimal.Decimal": "immutable by contract, every operation returns a new value",
			"[]byte":                                "payload, ownership transferred by convention",

			"net.Conn":        "net documents that multiple goroutines may invoke methods on a Conn simultaneously",
			"net.PacketConn":  "net documents that multiple goroutines may invoke methods on a PacketConn simultaneously",
			"net.Listener":    "net documents a Listener as safe for use by multiple goroutines",
			"net.TCPConn":     "net documents that multiple goroutines may invoke methods on a Conn simultaneously",
			"net.UDPConn":     "net documents that multiple goroutines may invoke methods on a Conn simultaneously",
			"net.TCPListener": "net documents a Listener as safe for use by multiple goroutines",
			"sync.Pool":       "a Pool is safe for use by multiple goroutines by contract",
			"sync.Map":        "a Map is safe for concurrent use by contract",
		},

		AllowPackages: map[string]string{
			"ergo.services/ergo/app/...": "framework internal traffic, not user code",
			"ergo.services/ergo/node":    "framework internal traffic, not user code",
			"ergo.services/ergo/net/...": "framework internal traffic, not user code",
		},
		AcceptIfaces: map[string]bool{"error": true},

		Blocking: []BlockingSurface{
			{Func: "time.Sleep", Why: "sleep", Transitive: true},
			{Recv: "sync.WaitGroup", Method: "Wait", Why: "waitgroup", Transitive: true},
			{Recv: "sync.Mutex", Method: "Lock", Why: "mutex"},
			{Recv: "sync.RWMutex", Method: "Lock", Why: "mutex"},
			{Recv: "sync.RWMutex", Method: "RLock", Why: "mutex"},
		},
		RoundTrips: defaultRoundTrips(),
		Senders:    defaultSenders(),
		Callbacks:  defaultCallbacks(),
		Tests:      true,
	}
}

func (c *Config) PackageAllowed(path string) (string, bool) {
	if reason, ok := c.AllowPackages[path]; ok {
		return reason, true
	}
	for pattern, reason := range c.AllowPackages {
		prefix, ok := strings.CutSuffix(pattern, "/...")
		if ok == false {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return reason, true
		}
	}
	return "", false
}

var (
	flagConfig     string
	flagConfigHash string
	flagTests      bool
	flagKeys       bool
)

func BindFlags(fs *flag.FlagSet) {
	fs.StringVar(&flagConfig, "config", "", "path to argus.yml, absolute or discovered by walking up")
	fs.StringVar(&flagConfigHash, "confighash", "", "digest of the effective config, present only to defeat the vet cache")
	fs.BoolVar(&flagTests, "tests", true, "analyze test variants")
	fs.BoolVar(&flagKeys, "keys", false, "append a baseline key to every finding, so a baseline can be built from the output")
}

var (
	configMu    sync.Mutex
	configCache = map[string]*Config{}
)

func LoadConfig(anchorFile string) (*Config, error) {
	path := flagConfig
	if path == "" {
		path = discover(anchorFile)
	}

	configMu.Lock()
	defer configMu.Unlock()

	if cfg, ok := configCache[path]; ok {
		return cfg, nil
	}

	if path == "" {
		cfg := DefaultConfig()
		cfg.Tests = flagTests
		configCache[path] = cfg
		return cfg, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("argus: resolve config path %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("argus: read config %q: %w", abs, err)
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("argus: parse config %q: %w", abs, err)
	}
	cfg.path = abs
	cfg.Tests = flagTests
	configCache[path] = cfg
	return cfg, nil
}

func ParseConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, err
	}
	cfg.path = path
	return cfg, nil
}

func discover(anchorFile string) string {
	if anchorFile == "" {
		return ""
	}
	dir := filepath.Dir(anchorFile)
	for {
		candidate := filepath.Join(dir, "argus.yml")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() == false {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func parseConfig(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	section, sub := "", ""
	var pendingPath string
	replaced := map[string]bool{}

	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if indent == 0 {
			key, value, _ := strings.Cut(trimmed, ":")
			section, sub, pendingPath = strings.TrimSpace(key), "", ""
			value = strings.TrimSpace(value)
			switch section {
			case "version":
				if value != "" && value != "1" {
					return nil, fmt.Errorf("line %d: unsupported version %q", i+1, value)
				}
			case "tiers":
				if err := parseInlineTiers(cfg, value, i+1); err != nil {
					return nil, err
				}
			case "allow", "interfaces", "surfaces", "unknown":
			case "baseline":
				cfg.Baseline = unquote(value)
			case "tests":
				cfg.Tests = value == "true"
			default:
				return nil, fmt.Errorf("line %d: unknown key %q", i+1, section)
			}
			continue
		}

		if strings.HasPrefix(trimmed, "-") == false {
			key, value, _ := strings.Cut(trimmed, ":")
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)

			switch section {
			case "tiers":
				var tier int
				if _, err := fmt.Sscanf(key, "tier%d", &tier); err != nil {
					return nil, fmt.Errorf("line %d: expected tierN, got %q", i+1, key)
				}
				sev, err := parseSeverity(value)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", i+1, err)
				}
				cfg.Tiers[tier] = sev
				continue
			case "allow":
				if key != "types" && key != "packages" {
					return nil, fmt.Errorf("line %d: unknown allow section %q, want types or packages", i+1, key)
				}
			case "interfaces":
				if key != "accept" {
					return nil, fmt.Errorf("line %d: unknown interfaces field %q, want accept", i+1, key)
				}
				if value != "" {
					cfg.AcceptIfaces = map[string]bool{}
					for _, name := range parseInlineList(value) {
						cfg.AcceptIfaces[name] = true
					}
				}
			case "surfaces":
				switch key {
				case "senders", "callbacks", "blocking", "roundtrip":
				default:
					if err := parseSurfaceContinuation(cfg, sub, key, value, i+1); err != nil {
						return nil, err
					}
					continue
				}
			case "":
				return nil, fmt.Errorf("line %d: indented key %q outside any section", i+1, key)
			}
			sub, pendingPath = key, ""
			continue
		}

		entry := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		switch section {
		case "allow":
			switch sub {
			case "types", "packages":
			default:
				return nil, fmt.Errorf("line %d: allow entry outside a types or packages section", i+1)
			}
			path, reason, err := parseAllowEntry(entry, &pendingPath, i+1)
			if err != nil {
				return nil, err
			}
			if path == "" {
				continue
			}
			target := cfg.AllowTypes
			if sub == "packages" {
				target = cfg.AllowPackages
			}
			if replaced[sub] == false {

				clear(target)
				replaced[sub] = true
			}
			target[path] = reason
		case "interfaces":
			if replaced["accept"] == false {
				cfg.AcceptIfaces = map[string]bool{}
				replaced["accept"] = true
			}
			cfg.AcceptIfaces[strings.Trim(entry, `"`)] = true
		case "surfaces":
			if err := parseSurfaceEntry(cfg, sub, entry, replaced, i+1); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("line %d: list item in section %q, which takes none", i+1, section)
		}
	}
	if pendingPath != "" {
		return nil, fmt.Errorf("allow entry %q has no reason", pendingPath)
	}
	return cfg, nil
}

func parseInlineTiers(cfg *Config, value string, line int) error {
	if value == "" {
		return nil
	}
	inner, ok := braced(value)
	if ok == false {
		return fmt.Errorf("line %d: expected tiers as a nested block or { tierN: severity }", line)
	}
	for _, part := range strings.Split(inner, ",") {
		key, sev, ok := strings.Cut(part, ":")
		if ok == false {
			continue
		}
		var tier int
		if _, err := fmt.Sscanf(strings.TrimSpace(key), "tier%d", &tier); err != nil {
			return fmt.Errorf("line %d: expected tierN, got %q", line, key)
		}
		s, err := parseSeverity(sev)
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		cfg.Tiers[tier] = s
	}
	return nil
}

func parseAllowEntry(entry string, pending *string, line int) (string, string, error) {
	if inner, ok := braced(entry); ok {
		head, reason, ok := strings.Cut(inner, "reason:")
		if ok == false {
			return "", "", fmt.Errorf("line %d: allow entry needs a reason", line)
		}
		path := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(head), ","))
		key, value, ok := strings.Cut(path, ":")
		if ok == false || strings.TrimSpace(key) != "path" {
			return "", "", fmt.Errorf("line %d: allow entry needs a path", line)
		}
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return "", "", fmt.Errorf("line %d: allow entry needs a reason", line)
		}
		return unquote(value), unquote(reason), nil
	}

	key, value, ok := strings.Cut(entry, ":")
	if ok == false {
		return "", "", fmt.Errorf("line %d: expected path or reason", line)
	}
	switch strings.TrimSpace(key) {
	case "path":
		*pending = unquote(value)
		return "", "", nil
	case "reason":
		if *pending == "" {
			return "", "", fmt.Errorf("line %d: reason without a preceding path", line)
		}
		reason := unquote(value)
		if reason == "" {
			return "", "", fmt.Errorf("line %d: allow entry needs a reason", line)
		}
		path := *pending
		*pending = ""
		return path, reason, nil
	}
	return "", "", fmt.Errorf("line %d: unknown allow field %q", line, key)
}

func parseSurfaceContinuation(cfg *Config, sub, key, value string, line int) error {
	if sub == "callbacks" && key == "methods" && len(cfg.Callbacks) > 0 {
		cfg.Callbacks[len(cfg.Callbacks)-1].Methods = parseInlineList(value)
		return nil
	}
	return fmt.Errorf("line %d: unknown surfaces section %q", line, key)
}

func parseSurfaceEntry(cfg *Config, sub, entry string, replaced map[string]bool, line int) error {
	inner, ok := braced(entry)
	if ok == false {

		key, value, cut := strings.Cut(entry, ":")
		if cut == false {
			return fmt.Errorf("line %d: expected a { } entry", line)
		}
		if sub != "callbacks" || strings.TrimSpace(key) != "recv" {
			return fmt.Errorf("line %d: expected a { } entry", line)
		}
		if replaced[sub] == false {
			cfg.Callbacks = nil
			replaced[sub] = true
		}
		cfg.Callbacks = append(cfg.Callbacks, CallbackSurface{Recv: unquote(value)})
		return nil
	}

	fields := map[string]string{}
	for _, part := range splitFields(inner) {
		key, value, ok := strings.Cut(part, ":")
		if ok == false {
			continue
		}
		fields[strings.TrimSpace(key)] = unquote(value)
	}

	if replaced[sub] == false {
		switch sub {
		case "senders":
			cfg.Senders = nil
		case "callbacks":
			cfg.Callbacks = nil
		case "blocking":
			cfg.Blocking = nil
		case "roundtrip":
			cfg.RoundTrips = nil
		}
		replaced[sub] = true
	}

	switch sub {
	case "senders":
		param, err := strconv.Atoi(fields["param"])
		if err != nil {
			return fmt.Errorf("line %d: sender entry needs a numeric param", line)
		}
		if fields["method"] == "" {
			return fmt.Errorf("line %d: sender entry needs a method", line)
		}
		cfg.Senders = append(cfg.Senders, SenderSurface{
			Recv: fields["recv"], Method: fields["method"], Param: param,
		})
	case "callbacks":
		cfg.Callbacks = append(cfg.Callbacks, CallbackSurface{
			Recv: fields["recv"], Methods: parseInlineList(fields["methods"]),
		})
	case "blocking":
		if fields["func"] == "" && fields["method"] == "" {
			return fmt.Errorf("line %d: blocking entry needs a func or a recv plus method", line)
		}

		transitive := fields["transitive"] != "false"
		cfg.Blocking = append(cfg.Blocking, BlockingSurface{
			Func: fields["func"], Recv: fields["recv"],
			Method: fields["method"], Why: fields["why"], Transitive: transitive,
		})
	case "roundtrip":
		if fields["func"] == "" && fields["method"] == "" {
			return fmt.Errorf("line %d: roundtrip entry needs a func or a recv plus method", line)
		}
		cfg.RoundTrips = append(cfg.RoundTrips, RoundTripSurface{
			Func: fields["func"], Recv: fields["recv"],
			Method: fields["method"], Why: fields["why"],
		})
	default:
		return fmt.Errorf("line %d: surfaces entry outside a known section", line)
	}
	return nil
}

func splitFields(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

func parseInlineList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := unquote(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func braced(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return s[1 : len(s)-1], true
	}
	return "", false
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}

func (c *Config) Severity(tier int) Severity {
	if s, ok := c.Tiers[tier]; ok {
		return s
	}
	return SeverityOff
}

func defaultRoundTrips() []RoundTripSurface {
	return []RoundTripSurface{
		{Recv: "net/http.Client", Method: "Do", Why: "an HTTP round trip"},
		{Recv: "net/http.Client", Method: "Get", Why: "an HTTP round trip"},
		{Recv: "net/http.Client", Method: "Post", Why: "an HTTP round trip"},
		{Recv: "net/http.Client", Method: "PostForm", Why: "an HTTP round trip"},
		{Recv: "net/http.Client", Method: "Head", Why: "an HTTP round trip"},
		{Func: "net/http.Get", Why: "an HTTP round trip"},
		{Func: "net/http.Post", Why: "an HTTP round trip"},
		{Func: "net/http.PostForm", Why: "an HTTP round trip"},
		{Func: "net/http.Head", Why: "an HTTP round trip"},
		{Recv: "database/sql.DB", Method: "Exec", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "ExecContext", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "Query", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "QueryContext", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "QueryRow", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "QueryRowContext", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "Ping", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "PingContext", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "Begin", Why: "a database round trip"},
		{Recv: "database/sql.DB", Method: "BeginTx", Why: "a database round trip"},
		{Recv: "database/sql.Tx", Method: "Commit", Why: "a database round trip"},
		{Recv: "database/sql.Tx", Method: "Exec", Why: "a database round trip"},
		{Recv: "database/sql.Tx", Method: "ExecContext", Why: "a database round trip"},
		{Recv: "database/sql.Tx", Method: "Query", Why: "a database round trip"},
		{Recv: "database/sql.Tx", Method: "QueryContext", Why: "a database round trip"},
		{Recv: "database/sql.Stmt", Method: "Exec", Why: "a database round trip"},
		{Recv: "database/sql.Stmt", Method: "ExecContext", Why: "a database round trip"},
		{Recv: "database/sql.Stmt", Method: "Query", Why: "a database round trip"},
		{Recv: "database/sql.Stmt", Method: "QueryContext", Why: "a database round trip"},
	}
}

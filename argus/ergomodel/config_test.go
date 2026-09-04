package ergomodel_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ergo.tools/argus/ergomodel"
)

func parse(t *testing.T, body string) (*ergomodel.Config, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "argus.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return ergomodel.ParseConfigFile(path)
}

func TestConfigTiersBothForms(t *testing.T) {
	nested, err := parse(t, "version: 1\ntiers:\n  tier1: error\n  tier2: off\n")
	if err != nil {
		t.Fatal(err)
	}
	if nested.Severity(1) != ergomodel.SeverityError {
		t.Errorf("tier1 = %v, want error", nested.Severity(1))
	}
	if nested.Severity(2) != ergomodel.SeverityOff {
		t.Errorf("tier2 = %v, want off", nested.Severity(2))
	}

	inline, err := parse(t, "tiers: { tier1: warn, tier3: error }\n")
	if err != nil {
		t.Fatal(err)
	}
	if inline.Severity(1) != ergomodel.SeverityWarn {
		t.Errorf("inline tier1 = %v, want warn", inline.Severity(1))
	}
	if inline.Severity(3) != ergomodel.SeverityError {
		t.Errorf("inline tier3 = %v, want error", inline.Severity(3))
	}
}

func TestConfigAllowReasonWithComma(t *testing.T) {
	cfg, err := parse(t, `allow:
  types:
    - { path: "[]byte", reason: payload, ownership transferred by convention }
`)
	if err != nil {
		t.Fatal(err)
	}
	reason, ok := cfg.AllowTypes["[]byte"]
	if ok == false {
		t.Fatal("[]byte entry missing")
	}
	if strings.Contains(reason, "ownership transferred") == false {
		t.Errorf("reason = %q, want the whole text including the part after the comma", reason)
	}
}

func TestConfigAllowTypesReplacesDefaults(t *testing.T) {
	cfg, err := parse(t, `allow:
  types:
    - { path: my/pkg.Handle, reason: interned }
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.AllowTypes["my/pkg.Handle"]; ok == false {
		t.Error("the configured entry is missing")
	}
	if _, ok := cfg.AllowTypes["time.Time"]; ok {
		t.Error("a configured allow list must replace the defaults, not extend them")
	}
}

func TestConfigAllowPackagesGlob(t *testing.T) {
	cfg, err := parse(t, `allow:
  packages:
    - { path: "example.com/gen/...", reason: generated }
    - { path: example.com/exact, reason: reviewed separately }
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"example.com/gen", "example.com/gen/sub/deep", "example.com/exact"} {
		if _, ok := cfg.PackageAllowed(path); ok == false {
			t.Errorf("%s should be allowlisted", path)
		}
	}
	for _, path := range []string{"example.com/generated", "example.com/exact/sub", "example.com/other"} {
		if _, ok := cfg.PackageAllowed(path); ok {
			t.Errorf("%s should not be allowlisted", path)
		}
	}
}

func TestConfigSurfaces(t *testing.T) {
	cfg, err := parse(t, `surfaces:
  senders:
    - { recv: my/pkg.Bus, method: Publish, param: 1 }
  blocking:
    - { func: my/pkg.WaitForever, why: custom wait }
  callbacks:
    - { recv: my/pkg.Behavior, methods: [Init, HandleMessage] }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Senders) != 1 || cfg.Senders[0].Method != "Publish" || cfg.Senders[0].Param != 1 {
		t.Errorf("senders = %+v, want the single configured entry", cfg.Senders)
	}
	if len(cfg.Blocking) != 1 || cfg.Blocking[0].Func != "my/pkg.WaitForever" {
		t.Errorf("blocking = %+v, want the single configured entry", cfg.Blocking)
	}
	if len(cfg.Callbacks) != 1 || len(cfg.Callbacks[0].Methods) != 2 {
		t.Errorf("callbacks = %+v, want one entry with two methods", cfg.Callbacks)
	}
}

func TestConfigMalformedIsAnError(t *testing.T) {
	cases := map[string]string{
		"unknown top level key":  "nonsense: 1\n",
		"unknown version":        "version: 2\n",
		"unknown severity":       "tiers:\n  tier1: loud\n",
		"allow without a reason": "allow:\n  types:\n    - { path: time.Time }\n",
		"reason before path":     "allow:\n  types:\n    - reason: orphan\n",
		"unknown allow section":  "allow:\n  colours:\n    - { path: x, reason: y }\n",
		"sender without param":   "surfaces:\n  senders:\n    - { recv: a, method: B }\n",
		"unknown surface":        "surfaces:\n  nonsense:\n    - { a: b }\n",
	}
	for name, body := range cases {
		if _, err := parse(t, body); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestDefaultConfigPolarity(t *testing.T) {
	cfg := ergomodel.DefaultConfig()
	if cfg.Severity(1) != ergomodel.SeverityError {
		t.Errorf("tier 1 severity = %v, want error", cfg.Severity(1))
	}
	if cfg.Severity(2) != ergomodel.SeverityWarn {
		t.Errorf("tier 2 severity = %v, want warn", cfg.Severity(2))
	}
	if cfg.Severity(3) != ergomodel.SeverityOff {
		t.Errorf("tier 3 severity = %v, want off by default", cfg.Severity(3))
	}
	if _, ok := cfg.AllowTypes["[]byte"]; ok == false {
		t.Error("[]byte must be allowlisted by default, it is the documented payload exception")
	}

	if _, ok := cfg.PackageAllowed("ergo.services/ergo/app/system/inspect"); ok == false {
		t.Error("the framework's own app tree must be excused by default")
	}
	if _, ok := cfg.PackageAllowed("example.com/user/app"); ok {
		t.Error("user code must not be excused by default")
	}
}

func TestConfigRoundTripSurface(t *testing.T) {
	cfg, err := parse(t, `version: 1
surfaces:
  roundtrip:
    - { recv: "example.com/app/ports.Repo", method: Load, why: "a database round trip" }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RoundTrips) != 1 {
		t.Fatalf("configured round trips = %d, want 1 replacing the defaults", len(cfg.RoundTrips))
	}
	got := cfg.RoundTrips[0]
	if got.Recv != "example.com/app/ports.Repo" || got.Method != "Load" {
		t.Errorf("round trip entry = %+v", got)
	}
}

func TestConfigCallbacksBothForms(t *testing.T) {
	inline, err := parse(t, `version: 1
surfaces:
  callbacks:
    - { recv: "example.com/app.Behavior", methods: [Init, HandleMessage] }
`)
	if err != nil {
		t.Fatal(err)
	}
	multiline, err := parse(t, `version: 1
surfaces:
  callbacks:
    - recv: "example.com/app.Behavior"
      methods: [Init, HandleMessage]
`)
	if err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]*ergomodel.Config{"inline": inline, "multiline": multiline} {
		if len(cfg.Callbacks) != 1 {
			t.Fatalf("%s: callbacks = %d, want 1", name, len(cfg.Callbacks))
		}
		got := cfg.Callbacks[0]
		if got.Recv != "example.com/app.Behavior" {
			t.Errorf("%s: recv = %q", name, got.Recv)
		}
		if len(got.Methods) != 2 || got.Methods[0] != "Init" || got.Methods[1] != "HandleMessage" {
			t.Errorf("%s: methods = %v", name, got.Methods)
		}
	}
}

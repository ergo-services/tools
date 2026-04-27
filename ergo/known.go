package main

// knownLoggers maps logger short names to their import paths.
var knownLoggers = map[string]string{
	"colored": "ergo.services/logger/colored",
	"rotate":  "ergo.services/logger/rotate",
}

// extraInfo holds metadata for a known extra application.
type extraInfo struct {
	Import string
	Create string
	Args   string
}

// knownExtras maps extra app short names to their metadata.
var knownExtras = map[string]extraInfo{
	"observer": {"ergo.services/application/observer", "CreateApp", "observer.Options{}"},
	"mcp":      {"ergo.services/application/mcp", "CreateApp", "mcp.Options{}"},
	"radar":    {"ergo.services/application/radar", "CreateApp", "radar.Options{}"},
}

// supType maps YAML supervisor type strings to Go constants.
func supType(t string) string {
	switch t {
	case "all_for_one":
		return "SupervisorTypeAllForOne"
	case "rest_for_one":
		return "SupervisorTypeRestForOne"
	case "simple_one_for_one":
		return "SupervisorTypeSimpleOneForOne"
	default:
		return "SupervisorTypeOneForOne"
	}
}

// supStrategy maps YAML strategy strings to Go constants.
func supStrategy(s string) string {
	switch s {
	case "permanent":
		return "SupervisorStrategyPermanent"
	case "temporary":
		return "SupervisorStrategyTemporary"
	default:
		return "SupervisorStrategyTransient"
	}
}

// appMode maps YAML mode strings to Go constants.
func appMode(m string) string {
	switch m {
	case "permanent":
		return "gen.ApplicationModePermanent"
	case "temporary":
		return "gen.ApplicationModeTemporary"
	default:
		return "gen.ApplicationModeTransient"
	}
}

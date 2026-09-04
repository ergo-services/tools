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

	// Hint is printed once by "ergo init" when the extra is part of the
	// generated project - typically the address its default options bind.
	// Empty means the extra has nothing to say for itself.
	Hint string
}

// knownExtras maps extra app short names to their metadata.
var knownExtras = map[string]extraInfo{
	// The observer serves the web UI, the API it runs on and the MCP surface for
	// an AI agent, all on one listener. There is no separate "mcp" extra.
	"observer": {
		Import: "ergo.services/application/observer",
		Create: "CreateApp",
		Args:   "observer.Options{}",
		Hint:   "Observer: http://localhost:9911 (MCP: http://localhost:9911/mcp)",
	},
	"radar": {
		Import: "ergo.services/application/radar",
		Create: "CreateApp",
		Args:   "radar.Options{}",
	},
}

// defaultExtras are added to a new project by "ergo init". The observer is
// what makes a fresh node worth opening: it serves the web UI and the agent
// interface, so "go run ./cmd" produces something to look at rather than a
// node that only logs. Remove the entry from ergo.yaml and re-run
// "ergo generate" to leave it out of an existing project.
var defaultExtras = []string{"observer"}

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

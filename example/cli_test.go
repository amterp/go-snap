package example

import (
	"testing"

	snap "github.com/amterp/go-snap"
)

// The common shape: a case is one invocation, and the suite declares what an
// invocation takes and what it produces.
var cliSuite = snap.Suite{
	Run: runCLI,
	Inputs: []snap.Input{
		// FIXTURE names a starting world. It is an ordinary input section -
		// the library has no fixture concept, because a name-to-func map in
		// the runner is already the whole feature.
		{Name: "FIXTURE"},
		// Quoted lets a case pass an empty or space-padded argument, which a
		// bare line-per-item encoding cannot express.
		{Name: "ARGS", List: true, Quoted: true},
		{Name: "STDIN"},
	},
	Outputs: []snap.Output{
		{Name: "STDOUT"},
		{Name: "STDERR"},
		{Name: "EXIT", Int: true},
	},
	Parallel: true,
}

// fixtures are worlds a case can ask for by name.
var fixtures = map[string]string{
	"":        "en",
	"french":  "fr",
	"german":  "de",
	"klingon": "tlh",
}

func TestCLISnapshots(t *testing.T) {
	snap.Run(t, "snapshots/cli", &cliSuite)
}

func runCLI(t *testing.T, c *snap.Case) {
	locale, ok := fixtures[c.Text("FIXTURE")]
	if !ok {
		t.Fatalf("unknown fixture %q - add it to fixtures in cli_test.go", c.Text("FIXTURE"))
	}

	stdout, stderr, exit := greet(locale, c.List("ARGS"), c.Text("STDIN"))

	c.Out("STDOUT", stdout)
	c.Out("STDERR", stderr)
	c.OutInt("EXIT", exit)
}

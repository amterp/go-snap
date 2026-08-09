package example

import (
	"testing"

	snap "github.com/amterp/go-snap"
)

// The same model with steps. Each action in a file is one interaction against
// one store, in order, and each may report its own result - which is what lets
// a case describe a session rather than a call.
var sessionSuite = snap.Suite{
	Run: runSession,
	Actions: []snap.Action{
		{Name: "SET", Args: 2},
		{Name: "GET", Args: 1},
		{Name: "DEL", Args: 1},
		// A body-carrying action, for values that do not fit on a header.
		{Name: "SET_BLOCK", Args: 1, HasBody: true},
	},
	Outputs: []snap.Output{
		// PerStep outputs are written directly under the action that produced
		// them, so a reader never has to count to find out which result
		// belongs to which step.
		{Name: "RESULT", PerStep: true},
		{Name: "FINAL"},
	},
	Parallel: true,
}

func TestSessionSnapshots(t *testing.T) {
	snap.Run(t, "snapshots/session", &sessionSuite)
}

func runSession(t *testing.T, c *snap.Case) {
	s := newStore()
	for _, step := range c.Steps() {
		switch step.Name {
		case "SET":
			step.Out("RESULT", s.set(step.Args[0], step.Args[1]))
		case "SET_BLOCK":
			step.Out("RESULT", s.set(step.Args[0], step.Body))
		case "GET":
			step.Out("RESULT", s.get(step.Args[0]))
		case "DEL":
			step.Out("RESULT", s.del(step.Args[0]))
		default:
			t.Fatalf("unhandled action %s", step.Name)
		}
	}
	c.Out("FINAL", s.dump())
}

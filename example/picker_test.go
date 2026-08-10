package example

import (
	"os"
	"testing"

	"github.com/amterp/color"
	snap "github.com/amterp/go-snap"
	"github.com/amterp/go-snap/prompt"
	"github.com/amterp/radish"
)

// Frames are compared as plain text, so color is off for the whole package.
// This is the determinism-by-construction rule in miniature: turn the
// nondeterminism off at the source rather than stripping escape codes back out
// of the snapshot afterwards.
func TestMain(m *testing.M) {
	color.NoColor = true
	os.Exit(m.Run())
}

var pickerSuite = snap.Suite{
	Run: runPicker,
	Inputs: []snap.Input{
		{Name: "OPTIONS", List: true},
		prompt.KeysSection,
	},
	Outputs: []snap.Output{
		{Name: "CHOICE"},
		prompt.FramesSection,
	},
	Parallel: true,
}

func TestPickerSnapshots(t *testing.T) {
	snap.Run(t, "snapshots/picker", &pickerSuite)
}

func runPicker(t *testing.T, c *snap.Case) {
	keys := c.List(prompt.KeysSection.Name)

	// A case with no KEYS gets the fail-loud driver rather than a real
	// terminal, so a missing script is an immediate error instead of a hang.
	var p prompter = prompt.NoKeys{}
	var script *radish.ScriptDriver
	if len(keys) > 0 {
		d, err := prompt.Driver(keys)
		if err != nil {
			t.Fatalf("%s: %v", prompt.KeysSection.Name, err)
		}
		p, script = d, d
	}

	choice, err := pickFood(p, c.List("OPTIONS"))
	if err != nil {
		choice = "error: " + err.Error()
	}
	c.Out("CHOICE", choice)

	// FRAMES is only asserted for cases that actually drove a prompt; a
	// channel the runner leaves unset asserts nothing.
	if script != nil {
		c.Out(prompt.FramesSection.Name, prompt.RenderFrames(script))
	}
}

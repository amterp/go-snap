package prompt

import (
	"strings"
	"testing"

	"github.com/amterp/radish"
)

func TestParseKeysHandlesNamedKeysAndLiterals(t *testing.T) {
	events, err := ParseKeys([]string{"down", `"ab"`, "space", "ctrl-c", "  ", "enter"})
	if err != nil {
		t.Fatalf("ParseKeys: %v", err)
	}
	want := []string{"down", "a", "b", "space", "ctrl-c", "enter"}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d", len(events), len(want))
	}
	for i, w := range want {
		if got := events[i].String(); got != w {
			t.Errorf("event %d = %q, want %q", i, got, w)
		}
	}
}

// The token language is only trustworthy if it is the exact inverse of what
// frame labels print - otherwise a snapshot could name a key no case can type.
func TestKeyVocabularyRoundTripsWithRadish(t *testing.T) {
	for _, kt := range []radish.KeyType{
		radish.KeyUp, radish.KeyDown, radish.KeyLeft, radish.KeyRight,
		radish.KeyEnter, radish.KeyBackspace, radish.KeyTab, radish.KeyShiftTab,
		radish.KeyEsc, radish.KeyHome, radish.KeyEnd, radish.KeyPageUp,
		radish.KeyPageDown, radish.KeyCtrlC, radish.KeyCtrlD, radish.KeyCtrlU,
	} {
		name := radish.KeyEvent(kt).String()
		events, err := ParseKeys([]string{name})
		if err != nil {
			t.Errorf("frame label %q is not a parseable token: %v", name, err)
			continue
		}
		if len(events) != 1 || events[0].String() != name {
			t.Errorf("%q did not round trip, got %v", name, events)
		}
	}
}

func TestParseKeysRejectsUnknownToken(t *testing.T) {
	_, err := ParseKeys([]string{"down", "wiggle"})
	if err == nil {
		t.Fatal("expected an error for an unknown key token")
	}
	if !strings.Contains(err.Error(), `"wiggle"`) {
		t.Errorf("error should name the offending token, got %v", err)
	}
}

func TestRenderFramesLabelsEachFrameByItsKeystroke(t *testing.T) {
	d, err := Driver([]string{"down", "enter"})
	if err != nil {
		t.Fatal(err)
	}
	m := radish.NewSelect().Title("Pick").Options("one", "two")
	if _, _, err := d.Run(m); err != nil {
		t.Fatal(err)
	}

	got := RenderFrames(d)
	for _, want := range []string{
		"--- frame 0 (initial) ---",
		"--- frame 1 (down) ---",
		"--- frame 2 (enter) ---",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("frames missing %q:\n%s", want, got)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("rendered frames should not end with a newline; the section adds it")
	}
}

func TestNoKeysFailsInsteadOfTouchingATerminal(t *testing.T) {
	_, _, err := NoKeys{}.Run(radish.NewSelect().Options("one"))
	if err == nil {
		t.Fatal("expected an error when a prompt is reached without keys")
	}
	if !strings.Contains(err.Error(), KeysSection.Name) {
		t.Errorf("error should name the section that is missing, got %v", err)
	}
}

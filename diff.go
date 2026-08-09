package snap

import (
	"fmt"
	"os"
	"strings"

	gd "github.com/amterp/go-delta"
)

// DiffConfig tunes how a mismatch is rendered. The goal a suite should hold it
// to: a failure is diagnosable from the test output alone, without opening the
// snapshot file to compare by hand.
type DiffConfig struct {
	// Width of the rendered diff. Default 120.
	Width int
	// Color forces color on or off. Nil auto-detects, honoring NO_COLOR and
	// SNAP_NO_COLOR.
	Color *bool
	// Func replaces the renderer entirely.
	Func func(want, got string) string
}

func (d DiffConfig) render(want, got string) string {
	if d.Func != nil {
		return d.Func(want, got)
	}
	width := d.Width
	if width == 0 {
		width = 120
	}
	opts := []gd.Option{gd.WithLayout(gd.LayoutPreferSideBySide), gd.WithWidth(width)}
	if c := d.colorMode(); c != nil {
		opts = append(opts, gd.WithColor(*c))
	}
	return gd.DiffWith(want, got, opts...)
}

func (d DiffConfig) colorMode() *bool {
	if d.Color != nil {
		return d.Color
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("SNAP_NO_COLOR") != "" {
		off := false
		return &off
	}
	return nil
}

// report renders one mismatch, ending with the file location it came from and
// the exact command that would accept it. Both matter most to the reader who
// did not write the test - increasingly an agent, for whom "run this to
// accept" beats any amount of prose.
func (d DiffConfig) report(m mismatch, relPath string) string {
	var b strings.Builder

	where := m.channel + " mismatch"
	if m.step != nil {
		where += " at " + m.step.label()
	}

	switch m.kind {
	case mmInt:
		fmt.Fprintf(&b, "%s: want %d, got %d\n", where, m.wantNum, m.gotNum)
	case mmStale:
		subject := "the case"
		if m.step != nil {
			subject = m.step.label()
		}
		fmt.Fprintf(&b, "%s is present in the file but the runner never set it for %s, so the section asserts nothing.\n"+
			"Set the channel in the runner, or delete the section.\n", m.channel, subject)
	default:
		want, got := m.want, m.got
		if m.raw {
			want, got = quoteForDiff(want), quoteForDiff(got)
		}
		fmt.Fprintf(&b, "%s:\n%s\n", where, d.render(want, got))
	}

	fmt.Fprintf(&b, "  at %s:%d\n", relPath, m.line)
	fmt.Fprintf(&b, "  to accept: -update=%s", relPath)
	return b.String()
}

// quoteForDiff renders a raw value one Go-quoted line per source line, so a
// difference in invisible bytes is visible in the diff while the diff itself
// stays line-oriented.
func quoteForDiff(s string) string {
	lines := strings.SplitAfter(s, "\n")
	if n := len(lines); n > 1 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = fmt.Sprintf("%q", line)
	}
	return strings.Join(out, "\n")
}

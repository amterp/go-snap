package snap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeReporter captures what the engine would have told the test framework.
type fakeReporter struct {
	errors  []string
	skipped string
	didSkip bool
}

func (f *fakeReporter) Helper() {}
func (f *fakeReporter) Errorf(format string, args ...any) {
	f.errors = append(f.errors, sprintf(format, args...))
}
func (f *fakeReporter) Skip(args ...any) {
	f.didSkip = true
	f.skipped = sprintln(args...)
}

func sprintf(format string, args ...any) string {
	return strings.TrimSuffix(fmt.Sprintf(format, args...), "\n")
}

func sprintln(args ...any) string { return fmt.Sprintln(args...) }

// runOne drives a single case the way Run would, capturing the report.
func runOne(t *testing.T, s *Suite, text string, updating bool) (*Case, *fakeReporter) {
	t.Helper()
	sc, err := s.schema()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := parse(sc, s.Markers, "snapshots/x.snap", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	rep := &fakeReporter{}
	runCase(t, rep, s, sc, cases[0], "snapshots/x.snap", updating)
	return cases[0], rep
}

// cliSuite is the shape most suites have: one input, three channels.
func cliSuite(run RunFunc) *Suite {
	off := false
	return &Suite{
		Run:     run,
		Inputs:  []Input{{Name: "ARGS", List: true, Quoted: true}},
		Outputs: []Output{{Name: "STDOUT"}, {Name: "STDERR"}, {Name: "EXIT", Int: true}},
		Diff:    DiffConfig{Color: &off},
	}
}

func TestMatchingCaseReportsNothing(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) {
		c.Out("STDOUT", "hello")
		c.OutInt("EXIT", 2)
	})
	_, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT ###\nhello\n### EXIT ###\n2\n", false)
	if len(rep.errors) != 0 {
		t.Errorf("expected no failures, got %q", rep.errors)
	}
}

func TestTextMismatchReportsDiffLocationAndAcceptCommand(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { c.Out("STDOUT", "goodbye") })
	_, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT ###\nhello\n", false)
	if len(rep.errors) != 1 {
		t.Fatalf("expected one failure, got %d: %q", len(rep.errors), rep.errors)
	}
	msg := rep.errors[0]
	for _, want := range []string{
		"STDOUT mismatch",
		"hello",
		"goodbye",
		"at snapshots/x.snap:3",
		"to accept: -update=snapshots/x.snap",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("failure message missing %q:\n%s", want, msg)
		}
	}
}

func TestIntMismatchReportsPlainValues(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { c.OutInt("EXIT", 1) })
	_, rep := runOne(t, s, "### TITLE ###\nx\n### EXIT ###\n2\n", false)
	if len(rep.errors) != 1 || !strings.Contains(rep.errors[0], "EXIT mismatch: want 2, got 1") {
		t.Errorf("got %q", rep.errors)
	}
}

func TestAbsentSectionAssertsTheZeroValue(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) {
		c.Out("STDOUT", "unexpected output")
		c.OutInt("EXIT", 0)
	})
	_, rep := runOne(t, s, "### TITLE ###\nx\n", false)
	if len(rep.errors) != 1 || !strings.Contains(rep.errors[0], "STDOUT mismatch") {
		t.Errorf("an absent section should assert emptiness, got %q", rep.errors)
	}
}

func TestUnsetChannelAssertsNothing(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { c.Out("STDOUT", "hello") })
	_, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT ###\nhello\n", false)
	if len(rep.errors) != 0 {
		t.Errorf("an unset channel with no section should assert nothing, got %q", rep.errors)
	}
}

func TestSectionPresentButNeverSetIsStale(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { c.Out("STDOUT", "hello") })
	_, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT ###\nhello\n### STDERR ###\nleftover\n", false)
	if len(rep.errors) != 1 {
		t.Fatalf("expected one failure, got %q", rep.errors)
	}
	if !strings.Contains(rep.errors[0], "STDERR is present in the file but the runner never set it") {
		t.Errorf("got %q", rep.errors[0])
	}
}

func TestTrailingNewlinesAreForgivenButRawIsExact(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { c.Out("STDOUT", "hello\n\n") })
	if _, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT ###\nhello\n", false); len(rep.errors) != 0 {
		t.Errorf("text channels should forgive trailing newlines, got %q", rep.errors)
	}
	if _, rep := runOne(t, s, "### TITLE ###\nx\n### STDOUT [raw] ###\n\"hello\"\n", false); len(rep.errors) != 1 {
		t.Errorf("a raw channel should compare exactly, got %q", rep.errors)
	}
}

func TestSkippedCaseNeverRuns(t *testing.T) {
	ran := false
	s := cliSuite(func(t *testing.T, c *Case) { ran = true })
	c, rep := runOne(t, s, "### TITLE ###\nx\n### SKIP ###\nupstream is flaky\n### STDOUT ###\nhello\n", false)
	if ran {
		t.Error("a skipped case must not run")
	}
	if !rep.didSkip || !strings.Contains(rep.skipped, "upstream is flaky") {
		t.Errorf("skip = %q", rep.skipped)
	}
	if c.ran {
		t.Error("a skipped case must not be marked as run, or an update would erase it")
	}
}

func TestPerStepOutputsAreCorrelated(t *testing.T) {
	s := &Suite{
		Run: func(t *testing.T, c *Case) {
			for _, st := range c.Steps() {
				st.Out("RESULT", "got-"+st.Args[0])
			}
		},
		Actions: []Action{{Name: "AT", Args: 1}},
		Outputs: []Output{{Name: "RESULT", PerStep: true}},
	}
	off := false
	s.Diff.Color = &off

	_, rep := runOne(t, s, "### TITLE ###\nx\n### AT 1 ###\n### RESULT ###\ngot-1\n### AT 2 ###\n### RESULT ###\nwrong\n", false)
	if len(rep.errors) != 1 {
		t.Fatalf("expected one failure, got %q", rep.errors)
	}
	if !strings.Contains(rep.errors[0], "RESULT mismatch at step 2 (AT 2)") {
		t.Errorf("a per-step failure should name its step, got %q", rep.errors[0])
	}
}

func TestUpdateRegeneratesOutputsAndKeepsHandAuthoredSections(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) {
		c.Out("STDOUT", "new output")
		c.OutInt("EXIT", 3)
	})
	src := "### TITLE ###\nx\n### DESCRIPTION ###\nwhy this exists\n### ARGS ###\n--flag\n\"\"\n### STDOUT ###\nold output\n### STDERR ###\nstale\n"
	c, rep := runOne(t, s, src, true)
	if len(rep.errors) != 0 {
		t.Errorf("an update run should not report mismatches, got %q", rep.errors)
	}

	sc, _ := s.schema()
	apply(sc, c)
	got := string(render(sc, s.Markers, []*Case{c}))
	want := "### TITLE ###\nx\n### DESCRIPTION ###\nwhy this exists\n### ARGS ###\n--flag\n\"\"\n### STDOUT ###\nnew output\n### EXIT ###\n3\n"
	if got != want {
		t.Errorf("update produced:\n%s\nwant:\n%s", got, want)
	}
}

func TestUpdateDoesNotEraseACaseWhoseRunnerBailedOut(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) { /* sets nothing, as if it had failed */ })
	sc, _ := s.schema()
	cases, err := parse(sc, s.Markers, "x.snap", []byte("### TITLE ###\nx\n### STDOUT ###\nkeep me\n"))
	if err != nil {
		t.Fatal(err)
	}
	// ran stays false, standing in for a runner that called t.Fatal.
	apply(sc, cases[0])
	if got := cases[0].expected["STDOUT"].text; got != "keep me" {
		t.Errorf("expectation was erased: %q", got)
	}
}

func TestRewriteMigratesFormattingWithNoBehaviorChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.snap")
	// Semantically identical to canonical form, but laid out differently:
	// sections out of declaration order and a missing separator line.
	src := "### TITLE ###\nx\n### EXIT ###\n1\n### STDOUT ###\nhello\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s := cliSuite(func(t *testing.T, c *Case) {})
	sc, _ := s.schema()
	cases, err := parse(sc, s.Markers, path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	st := &fileState{path: path, orig: []byte(src), cases: cases, updating: true}
	written, err := st.rewrite(sc, s.Markers)
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("expected the file to be reformatted")
	}
	got, _ := os.ReadFile(path)
	want := "### TITLE ###\nx\n### STDOUT ###\nhello\n### EXIT ###\n1\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteIsANoOpWhenNothingChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.snap")
	src := "### TITLE ###\nx\n### STDOUT ###\nhello\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	s := cliSuite(func(t *testing.T, c *Case) { c.Out("STDOUT", "hello") })
	sc, _ := s.schema()
	cases, _ := parse(sc, s.Markers, path, []byte(src))
	cases[0].ran = true
	cases[0].actual["STDOUT"] = &outVal{text: "hello"}

	st := &fileState{path: path, orig: []byte(src), cases: cases, updating: true}
	written, err := st.rewrite(sc, s.Markers)
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Error("an unchanged file should not be rewritten")
	}
}

func TestAccessorMisuseNamesTheDeclaration(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) {})
	sc, _ := s.schema()
	cases, _ := parse(sc, s.Markers, "x.snap", []byte("### TITLE ###\nx\n"))
	c := cases[0]

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic for an undeclared section")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, `"NOPE" is not a declared Input`) {
			t.Errorf("panic = %v", r)
		}
	}()
	c.Text("NOPE")
}

func TestAccessorMisuseAcrossKindsIsExplained(t *testing.T) {
	s := cliSuite(func(t *testing.T, c *Case) {})
	sc, _ := s.schema()
	cases, _ := parse(sc, s.Markers, "x.snap", []byte("### TITLE ###\nx\n"))

	defer func() {
		r := recover()
		msg, _ := r.(string)
		if !strings.Contains(msg, "it is declared as an Output") {
			t.Errorf("panic = %v", r)
		}
	}()
	cases[0].Text("STDOUT")
}

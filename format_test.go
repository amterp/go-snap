package snap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testSuite exercises every schema feature at once, so format tests do not
// each need their own suite.
func testSuite() *Suite {
	return &Suite{
		Run: func(t *testing.T, c *Case) {},
		Inputs: []Input{
			{Name: "INPUT"},
			{Name: "ARGS", List: true, Quoted: true},
			{Name: "KEYS", List: true},
			{Name: "BYTES", Raw: true},
		},
		Actions: []Action{
			{Name: "HOVER", Args: 1},
			{Name: "RENAME", Args: 2},
			{Name: "CHANGE", HasBody: true},
			{Name: "REFS", Args: -1},
			{Name: "TICK"},
		},
		Outputs: []Output{
			{Name: "STDOUT"},
			{Name: "STDERR"},
			{Name: "EXIT", Int: true},
			{Name: "RESULT", PerStep: true},
			{Name: "BYTES_OUT", Raw: true},
		},
	}
}

func parseString(t *testing.T, s *Suite, text string) []*Case {
	t.Helper()
	sc, err := s.schema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	cases, err := parse(sc, s.Markers, "test.snap", []byte(text))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return cases
}

func parseErr(t *testing.T, s *Suite, text string) string {
	t.Helper()
	sc, err := s.schema()
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	_, err = parse(sc, s.Markers, "test.snap", []byte(text))
	if err == nil {
		t.Fatalf("expected a parse error, got none")
	}
	return err.Error()
}

func TestParseBasicCase(t *testing.T) {
	cases := parseString(t, testSuite(), `### TITLE ###
hello
### DESCRIPTION ###
why this case exists
### INPUT ###
line one
line two
### STDOUT ###
out
### EXIT ###
2
`)
	if len(cases) != 1 {
		t.Fatalf("got %d cases, want 1", len(cases))
	}
	c := cases[0]
	if c.Title() != "hello" {
		t.Errorf("title = %q", c.Title())
	}
	if c.Description() != "why this case exists" {
		t.Errorf("description = %q", c.Description())
	}
	if got := c.Text("INPUT"); got != "line one\nline two" {
		t.Errorf("INPUT = %q", got)
	}
	if got := c.expected["STDOUT"].text; got != "out" {
		t.Errorf("STDOUT = %q", got)
	}
	if got := c.expected["EXIT"].num; got != 2 {
		t.Errorf("EXIT = %d", got)
	}
}

func TestParseEmptyVersusAbsentInput(t *testing.T) {
	cases := parseString(t, testSuite(), `### TITLE ###
present but empty
### INPUT ###
### STDOUT ###
x

### TITLE ###
absent
### STDOUT ###
x
`)
	if !cases[0].Has("INPUT") {
		t.Error("empty INPUT should be present")
	}
	if got := cases[0].Text("INPUT"); got != "" {
		t.Errorf("empty INPUT = %q, want empty", got)
	}
	if cases[1].Has("INPUT") {
		t.Error("omitted INPUT should be absent")
	}
}

// TestTrailingNewlinesAreCounted pins the rule that makes an input faithful: a
// body line is terminated by its newline, and the last of those is the
// separator before the next header. So each blank line before that header is
// one more trailing newline in the value.
//
// An input is data the runner feeds to the system under test, not a claim about
// it. A section that silently dropped trailing bytes would hand the runner
// something the file does not say - and the difference is invisible in review,
// because what is missing is whitespace.
func TestTrailingNewlinesAreCounted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		blank string
		want  string
	}{
		{"none", "", "foo"},
		{"one", "\n", "foo\n"},
		{"two", "\n\n", "foo\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "### TITLE ###\nt\n### INPUT ###\nfoo\n" + tc.blank + "### STDOUT ###\nx\n"
			if got := parseString(t, testSuite(), text)[0].Text("INPUT"); got != tc.want {
				t.Errorf("INPUT = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTrailingNewlinesSurviveARewrite is the other half: accepting an update
// anywhere in a file rewrites the whole file, so a hand-authored trailing
// newline has to round trip. Losing it here would corrupt an input that had
// nothing to do with the change being accepted.
func TestTrailingNewlinesSurviveARewrite(t *testing.T) {
	const text = "### TITLE ###\nt\n### INPUT ###\nfoo\n\n### STDOUT ###\nx\n"
	s := testSuite()
	cases := parseString(t, s, text)

	path := filepath.Join(t.TempDir(), "out.snap")
	if err := WriteFile(s, path, cases); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != text {
		t.Errorf("round trip changed the file:\n got %q\nwant %q", got, text)
	}
}

// TestCaseSeparatorIsNotContent covers the one place the rule above needs an
// exception: the writer puts a blank line between cases, so a single blank line
// before a TITLE is structure. A body that really ends in a blank line writes
// two, and the count still works.
func TestCaseSeparatorIsNotContent(t *testing.T) {
	cases := parseString(t, testSuite(), `### TITLE ###
separator only
### INPUT ###
foo

### TITLE ###
separator plus a real trailing newline
### INPUT ###
bar


### TITLE ###
last in file
### INPUT ###
baz
`)
	for i, want := range []string{"foo", "bar\n", "baz"} {
		if got := cases[i].Text("INPUT"); got != want {
			t.Errorf("case %d INPUT = %q, want %q", i, got, want)
		}
	}
}

func TestParseQuotedListPreservesEdgeItems(t *testing.T) {
	c := parseString(t, testSuite(), `### TITLE ###
args
### ARGS ###
--flag
""
"  padded  "
plain
### STDOUT ###
x
`)[0]
	want := []string{"--flag", "", "  padded  ", "plain"}
	got := c.List("ARGS")
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseStepsWithArgsAndPerStepOutputs(t *testing.T) {
	c := parseString(t, testSuite(), `### TITLE ###
session
### INPUT ###
doc
### HOVER 1:6 ###
### RESULT ###
first
### RENAME 0:0 bar ###
### RESULT ###
second
### CHANGE ###
new text
### REFS 1:2 decl extra ###
### STDOUT ###
done
`)[0]
	steps := c.Steps()
	if len(steps) != 4 {
		t.Fatalf("got %d steps, want 4", len(steps))
	}
	if steps[0].Name != "HOVER" || len(steps[0].Args) != 1 || steps[0].Args[0] != "1:6" {
		t.Errorf("step 0 = %+v", steps[0])
	}
	if steps[0].expected["RESULT"].text != "first" {
		t.Errorf("step 0 RESULT = %q", steps[0].expected["RESULT"].text)
	}
	if steps[1].Args[1] != "bar" {
		t.Errorf("step 1 args = %q", steps[1].Args)
	}
	if steps[2].Body != "new text" {
		t.Errorf("step 2 body = %q", steps[2].Body)
	}
	if len(steps[3].Args) != 3 {
		t.Errorf("variadic step args = %q", steps[3].Args)
	}
	if steps[3].label() != "step 4 (REFS 1:2 decl extra)" {
		t.Errorf("label = %q", steps[3].label())
	}
}

func TestParseRawSectionKeepsBytes(t *testing.T) {
	c := parseString(t, testSuite(), `### TITLE ###
raw
### BYTES ###
"a\tb  \r\n"
### BYTES_OUT ###
"trailing \n\n"
`)[0]
	if got := c.Text("BYTES"); got != "a\tb  \r\n" {
		t.Errorf("BYTES = %q", got)
	}
	if got := c.expected["BYTES_OUT"].text; got != "trailing \n\n" {
		t.Errorf("BYTES_OUT = %q", got)
	}
}

func TestParsePerInstanceRawFlag(t *testing.T) {
	c := parseString(t, testSuite(), `### TITLE ###
instance raw
### STDOUT [raw] ###
"exact  \n"
`)[0]
	v := c.expected["STDOUT"]
	if !v.raw {
		t.Fatal("STDOUT should be raw for this instance")
	}
	if v.text != "exact  \n" {
		t.Errorf("STDOUT = %q", v.text)
	}
}

func TestParseEscapedLines(t *testing.T) {
	c := parseString(t, testSuite(), `### TITLE ###
escapes
### STDOUT ###
normal
\"### TITLE ###"
\"trailing   "
`)[0]
	want := "normal\n### TITLE ###\ntrailing   "
	if got := c.expected["STDOUT"].text; got != want {
		t.Errorf("STDOUT = %q, want %q", got, want)
	}
}

func TestParseCustomMarkers(t *testing.T) {
	s := testSuite()
	s.Markers = Markers{Prefix: "=== ", Suffix: " ==="}
	c := parseString(t, s, `=== TITLE ===
custom
=== STDOUT ===
### TITLE ###
`)[0]
	if c.Title() != "custom" {
		t.Errorf("title = %q", c.Title())
	}
	// The default markers carry no meaning under a custom syntax, so a line
	// that looks like one is ordinary content.
	if got := c.expected["STDOUT"].text; got != "### TITLE ###" {
		t.Errorf("STDOUT = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"unknown section",
			"### TITLE ###\nx\n### STDOUR ###\ny\n",
			`test.snap:3: unknown section "STDOUR"`,
		},
		{
			"header arguments on a meta section",
			"### TITLE oops ###\nx\n### STDOUT ###\ny\n",
			`test.snap:1: TITLE takes no header arguments, got "oops"`,
		},
		{
			"raw flag on a meta section",
			"### TITLE [raw] ###\n\"x\"\n### STDOUT ###\ny\n",
			"test.snap:1: TITLE holds one line of text, so [raw] is meaningless",
		},
		{
			"duplicate description",
			"### TITLE ###\nx\n### DESCRIPTION ###\nfirst\n### DESCRIPTION ###\nsecond\n",
			"test.snap:5: DESCRIPTION appears twice in one case",
		},
		{
			"raw flag on a list input",
			"### TITLE ###\nx\n### ARGS [raw] ###\na\n",
			"test.snap:3: ARGS is a List, whose items are quoted per line, so [raw] is meaningless",
		},
		{
			"raw flag on an int output",
			"### TITLE ###\nx\n### EXIT [raw] ###\n1\n",
			"test.snap:3: EXIT is an Int, so [raw] is meaningless",
		},
		{
			"content before first title",
			"stray\n### TITLE ###\nx\n",
			"test.snap:1: content before the first TITLE",
		},
		{
			"duplicate title",
			"### TITLE ###\nx\n### STDOUT ###\na\n\n### TITLE ###\nx\n",
			`test.snap:6: duplicate title "x"`,
		},
		{
			"empty title",
			"### TITLE ###\n### STDOUT ###\na\n",
			"test.snap:1: TITLE is empty",
		},
		{
			"multi-line title",
			"### TITLE ###\na\nb\n### STDOUT ###\nc\n",
			"test.snap:1: TITLE: expected a single line, got 2",
		},
		{
			"description after input",
			"### TITLE ###\nx\n### INPUT ###\na\n### DESCRIPTION ###\nd\n",
			"test.snap:5: DESCRIPTION must come before",
		},
		{
			"input after step",
			"### TITLE ###\nx\n### TICK ###\n### INPUT ###\na\n",
			"test.snap:4: input INPUT must come before",
		},
		{
			"duplicate input",
			"### TITLE ###\nx\n### INPUT ###\na\n### INPUT ###\nb\n",
			"test.snap:5: input INPUT appears twice",
		},
		{
			"per-step output with no step",
			"### TITLE ###\nx\n### RESULT ###\na\n",
			"test.snap:3: output RESULT is declared PerStep but no action precedes it",
		},
		{
			"wrong arg count",
			"### TITLE ###\nx\n### HOVER ###\n",
			"test.snap:3: action HOVER takes 1 header argument(s), got 0",
		},
		{
			"body on bodiless action",
			"### TITLE ###\nx\n### TICK ###\nbody\n",
			"test.snap:3: action TICK takes no body",
		},
		{
			"non-integer int output",
			"### TITLE ###\nx\n### EXIT ###\nnope\n",
			`test.snap:3: output EXIT is declared Int but holds "nope"`,
		},
		{
			"duplicate output",
			"### TITLE ###\nx\n### STDOUT ###\na\n### STDOUT ###\nb\n",
			"test.snap:5: output STDOUT appears twice",
		},
		{
			"input with header args",
			"### TITLE ###\nx\n### INPUT 1:2 ###\na\n",
			"test.snap:3: input INPUT takes no header arguments",
		},
		{
			"malformed raw body",
			"### TITLE ###\nx\n### BYTES ###\nnot quoted\n",
			"holds a Go-quoted string",
		},
		{
			"no cases",
			"\n\n",
			"no cases",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseErr(t, testSuite(), tt.in)
			if !strings.Contains(got, tt.want) {
				t.Errorf("error = %q\nwant it to contain %q", got, tt.want)
			}
		})
	}
}

// TestUnknownSectionErrorSuggestsEscape checks the error tells the author how
// to keep the line as content, which is the other thing they might have meant.
func TestUnknownSectionErrorSuggestsEscape(t *testing.T) {
	got := parseErr(t, testSuite(), "### TITLE ###\nx\n### NOPE ###\n")
	if !strings.Contains(got, `\"### NOPE ###"`) {
		t.Errorf("error should suggest the escaped form, got %q", got)
	}
}

func TestWriteRoundTripIsStable(t *testing.T) {
	s := testSuite()
	sc, err := s.schema()
	if err != nil {
		t.Fatal(err)
	}
	src := `### TITLE ###
first
### DESCRIPTION ###
prose
about two lines
### SKIP ###
flaky upstream
### INPUT ###
some input

with a blank line
### ARGS ###
--flag
""
### BYTES ###
"raw\tcontent\r\n"
### STDOUT ###
output
\"### TITLE ###"
### EXIT ###
3

### TITLE ###
second
### KEYS ###
down
enter
### HOVER 1:6 ###
### RESULT ###
hovered
### CHANGE ###
changed text
### RESULT ###
changed
### STDERR ###
oops
`
	cases, err := parse(sc, s.Markers, "test.snap", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	first := render(sc, s.Markers, cases)

	again, err := parse(sc, s.Markers, "test.snap", first)
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, first)
	}
	second := render(sc, s.Markers, again)

	if string(first) != string(second) {
		t.Errorf("render is not stable:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if string(first) != src {
		t.Errorf("canonical form differs from source:\n--- got ---\n%s\n--- want ---\n%s", first, src)
	}
}

func TestWriteEscapesUnrepresentableLines(t *testing.T) {
	s := testSuite()
	sc, _ := s.schema()
	c := newCase(sc, "test.snap")
	c.title = "escaping"
	c.expected["STDOUT"] = &outVal{text: "ok\n### TITLE ###\ntrailing  \n\\\"already\""}

	out := string(render(sc, s.Markers, []*Case{c}))
	round, err := parse(sc, s.Markers, "test.snap", []byte(out))
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, out)
	}
	if got := round[0].expected["STDOUT"].text; got != c.expected["STDOUT"].text {
		t.Errorf("round trip lost content:\ngot  %q\nwant %q", got, c.expected["STDOUT"].text)
	}
}

func TestSkipSectionPresenceIsMeaningful(t *testing.T) {
	cases := parseString(t, testSuite(), `### TITLE ###
skipped
### SKIP ###
because
### STDOUT ###
x

### TITLE ###
not skipped
### STDOUT ###
x
`)
	if cases[0].skip == nil || *cases[0].skip != "because" {
		t.Errorf("skip = %v", cases[0].skip)
	}
	if cases[1].skip != nil {
		t.Error("second case should not be skipped")
	}
}

func TestDiscoverIgnoresPendingFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("### TITLE ###\nx\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.snap")
	write("b.snap.new")
	write("c.txt")

	paths, err := discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || filepath.Base(paths[0]) != "a.snap" {
		t.Errorf("discover = %v, want only a.snap", paths)
	}
}

// A list item is hand-authored content that a rewrite must return unchanged.
// Every case here was a way the writer used to lose or duplicate one, and the
// last two do worse than lose it: they splice a section into the case and leave
// a file that no longer parses.
func TestListItemsSurviveARewrite(t *testing.T) {
	s := testSuite()
	sc, err := s.schema()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		body string
		want []string
	}{
		{"embedded newline", `"a` + "\\n" + `b"`, []string{"a\nb"}},
		{"escape prefix", `"\\\"foo\""`, []string{`\"foo"`}},
		{"looks like a header", `"### STDOUT ###"`, []string{"### STDOUT ###"}},
		{"looks like a title", `"### TITLE ###"`, []string{"### TITLE ###"}},
		{"padded and empty", "\"  x  \"\n\"\"", []string{"  x  ", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "### TITLE ###\nx\n### ARGS ###\n" + tt.body + "\n### STDOUT ###\ny\n"
			cases, err := parse(sc, s.Markers, "test.snap", []byte(src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			assertItems(t, "on read", cases[0].List("ARGS"), tt.want)

			out := render(sc, s.Markers, cases)
			round, err := parse(sc, s.Markers, "test.snap", out)
			if err != nil {
				t.Fatalf("the rewritten file no longer parses: %v\n%s", err, out)
			}
			if len(round) != 1 {
				t.Fatalf("the rewrite produced %d cases from one:\n%s", len(round), out)
			}
			assertItems(t, "after a rewrite", round[0].List("ARGS"), tt.want)
			if got := round[0].expected["STDOUT"].text; got != "y" {
				t.Errorf("STDOUT became %q; the list spilled into the sections after it:\n%s", got, out)
			}
		})
	}
}

// A non-Quoted list carries no escape convention, so a line that looks escaped
// is just those characters. The writer emits it back verbatim rather than
// decoding it into something it then cannot represent.
func TestNonQuotedListIsPlainText(t *testing.T) {
	s := testSuite()
	sc, _ := s.schema()
	src := "### TITLE ###\nx\n### KEYS ###\n" + `\"### TITLE ###"` + "\nenter\n### STDOUT ###\ny\n"
	cases, err := parse(sc, s.Markers, "test.snap", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	assertItems(t, "on read", cases[0].List("KEYS"), []string{`\"### TITLE ###"`, "enter"})

	out := render(sc, s.Markers, cases)
	round, err := parse(sc, s.Markers, "test.snap", out)
	if err != nil {
		t.Fatalf("the rewritten file no longer parses: %v\n%s", err, out)
	}
	if len(round) != 1 {
		t.Fatalf("the rewrite invented %d cases:\n%s", len(round), out)
	}
	assertItems(t, "after a rewrite", round[0].List("KEYS"), []string{`\"### TITLE ###"`, "enter"})
}

func assertItems(t *testing.T, when string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d items %q, want %d %q", when, len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: item %d = %q, want %q", when, i, got[i], want[i])
		}
	}
}

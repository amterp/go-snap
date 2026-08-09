package snap

import (
	"strings"
	"testing"
)

// FuzzRoundTrip asserts the format's central law: rendering a parsed file and
// parsing it again yields the same bytes.
//
// The law is stated over already-canonical input rather than arbitrary text
// because the first render is what canonicalizes. What it catches is a value
// the writer cannot express and the parser therefore reads back differently -
// a body line that re-reads as a header, whitespace an editor would eat, a
// quoting rule that does not invert. Those are exactly the bugs that corrupt a
// snapshot file the first time someone accepts an update, silently.
func FuzzRoundTrip(f *testing.F) {
	seeds := []string{
		"### TITLE ###\nx\n### STDOUT ###\nhello\n",
		"### TITLE ###\nx\n### STDOUT ###\n### TITLE ###\n",
		"### TITLE ###\nx\n### STDOUT ###\ntrailing   \n",
		"### TITLE ###\nx\n### STDOUT ###\n\\\"already escaped\"\n",
		"### TITLE ###\nx\n### BYTES ###\n\"a\\tb\\r\\n\"\n",
		"### TITLE ###\nx\n### ARGS ###\n\"\"\n\"  \"\n--flag\n",
		"### TITLE ###\nx\n### INPUT ###\n\n\nblank lines\n\n### STDOUT ###\ny\n",
		"### TITLE ###\nx\n### HOVER 1:2 ###\n### RESULT ###\nr\n### CHANGE ###\nbody\n",
		"### TITLE ###\na\n### STDOUT ###\n1\n\n### TITLE ###\nb\n### EXIT ###\n2\n",
		"### TITLE ###\nx\n### STDOUT [raw] ###\n\"exact  \\n\"\n",
		"### TITLE ###\nx\n### STDOUT ###\n\u00e9\u4e2d\U0001f600\n",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	suite := testSuite()
	sc, err := suite.schema()
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, data string) {
		cases, err := parse(sc, suite.Markers, "fuzz.snap", []byte(data))
		if err != nil {
			return // not a valid file; the parser rejecting it is the point
		}
		first := render(sc, suite.Markers, cases)

		again, err := parse(sc, suite.Markers, "fuzz.snap", first)
		if err != nil {
			t.Fatalf("rendered output does not parse: %v\n--- rendered ---\n%s", err, first)
		}
		second := render(sc, suite.Markers, again)

		if string(first) != string(second) {
			t.Fatalf("round trip is not stable:\n--- first ---\n%q\n--- second ---\n%q", first, second)
		}

		// Values must survive the trip, not just the byte layout.
		if len(cases) != len(again) {
			t.Fatalf("case count changed: %d -> %d", len(cases), len(again))
		}
		for i := range cases {
			assertSameCase(t, cases[i], again[i])
		}
	})
}

func assertSameCase(t *testing.T, a, b *Case) {
	t.Helper()
	if a.title != b.title {
		t.Fatalf("title changed: %q -> %q", a.title, b.title)
	}
	if a.description != b.description {
		t.Fatalf("%s: description changed: %q -> %q", a.title, a.description, b.description)
	}
	for name, av := range a.inputs {
		bv, ok := b.inputs[name]
		if !ok {
			t.Fatalf("%s: input %s disappeared", a.title, name)
		}
		if av.text != bv.text || strings.Join(av.items, "\x00") != strings.Join(bv.items, "\x00") {
			t.Fatalf("%s: input %s changed: %q %q -> %q %q",
				a.title, name, av.text, av.items, bv.text, bv.items)
		}
	}
	if len(a.steps) != len(b.steps) {
		t.Fatalf("%s: step count changed: %d -> %d", a.title, len(a.steps), len(b.steps))
	}
	for i := range a.steps {
		if a.steps[i].Name != b.steps[i].Name || a.steps[i].Body != b.steps[i].Body {
			t.Fatalf("%s: step %d changed", a.title, i)
		}
		if strings.Join(a.steps[i].Args, " ") != strings.Join(b.steps[i].Args, " ") {
			t.Fatalf("%s: step %d args changed: %q -> %q", a.title, i, a.steps[i].Args, b.steps[i].Args)
		}
	}
	for name, av := range a.expected {
		bv, ok := b.expected[name]
		if !ok {
			// The writer drops a default-valued channel on purpose; an absent
			// section and a zero value mean the same thing.
			if av.text == "" && av.num == 0 {
				continue
			}
			t.Fatalf("%s: output %s disappeared with value %q", a.title, name, av.text)
		}
		if av.text != bv.text || av.num != bv.num {
			t.Fatalf("%s: output %s changed: %q/%d -> %q/%d",
				a.title, name, av.text, av.num, bv.text, bv.num)
		}
	}
}

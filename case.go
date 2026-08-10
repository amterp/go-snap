package snap

import (
	"fmt"
	"strings"
)

// Case is one snapshot case: its hand-authored inputs and steps, and the
// output channels the runner sets.
type Case struct {
	sc *schema

	title       string
	description string
	skip        *string // nil when the case has no SKIP section

	inputs map[string]*inputVal
	steps  []*Step

	// expected holds case-scoped output sections as read from the file.
	// Presence is meaningful: an absent section asserts the channel's zero
	// value, a present one that the runner never sets is a stale expectation.
	expected map[string]*outVal
	actual   map[string]*outVal

	// lines maps a section name to the 1-based line of its header, so a
	// mismatch can point at the exact spot in the file.
	lines map[string]int

	titleLine int
	file      string // path as given to Run, for messages
	t         caseT
	ran       bool
}

// caseT is the part of *testing.T the accessors need. It is never nil: a Case
// built by ParseFile carries noTest instead, so every accessor can call
// t.Helper() as one unconditional line. That matters more than it looks -
// Helper marks the frame that calls it, so hiding it behind a nil check in some
// shared helper would mark the helper and leave misuse messages pointing into
// go-snap, which is the whole problem it exists to solve.
type caseT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// noTest stands in when a Case exists outside a run. There is no test to fail,
// so misuse panics rather than returning a wrong answer to a tool.
type noTest struct{}

func (noTest) Helper() {}

func (noTest) Fatalf(format string, args ...any) {
	panic("snap: " + fmt.Sprintf(format, args...))
}

// Step is one occurrence of an Action within a case.
type Step struct {
	// Name is the action's section name.
	Name string
	// Args are the header argument tokens, already split.
	Args []string
	// Body is the section body, empty for header-only actions.
	Body string

	idx      int
	raw      bool
	line     int
	c        *Case
	expected map[string]*outVal
	actual   map[string]*outVal
	lines    map[string]int
}

type inputVal struct {
	text  string
	items []string
	raw   bool
}

type outVal struct {
	text string
	num  int
	raw  bool
}

// Title is the case's TITLE, unique within its file.
func (c *Case) Title() string { return c.title }

// Description is the case's DESCRIPTION, or "" if it has none. It is prose for
// humans; the engine never interprets it.
func (c *Case) Description() string { return c.description }

// Has reports whether the case declares the given input section at all,
// distinguishing a section present but empty from one that is absent.
func (c *Case) Has(section string) bool {
	c.t.Helper()
	c.mustInput(section)
	_, ok := c.inputs[section]
	return ok
}

// Text returns an input section's body, or "" if absent.
func (c *Case) Text(section string) string {
	c.t.Helper()
	in := c.mustInput(section)
	if in.List {
		c.fail("section %q is declared List; use List(%q) instead of Text", section, section)
	}
	if v, ok := c.inputs[section]; ok {
		return v.text
	}
	return ""
}

// List returns an input section's items, or nil if absent.
func (c *Case) List(section string) []string {
	c.t.Helper()
	in := c.mustInput(section)
	if !in.List {
		c.fail("section %q is not declared List; use Text(%q) instead", section, section)
	}
	if v, ok := c.inputs[section]; ok {
		return v.items
	}
	return nil
}

// Steps returns the case's action steps in file order.
func (c *Case) Steps() []*Step { return c.steps }

// Out records a case-scoped output channel. Setting a channel asserts it;
// leaving it unset asserts nothing. Setting it to "" asserts that it is empty.
func (c *Case) Out(channel, value string) {
	c.t.Helper()
	o := c.checkOutput(channel, false, false)
	c.actual[channel] = &outVal{text: value, raw: o.Raw}
}

// OutInt records an integer output channel.
func (c *Case) OutInt(channel string, value int) {
	c.t.Helper()
	c.checkOutput(channel, false, true)
	c.actual[channel] = &outVal{num: value}
}

// Out records a per-step output channel for this step.
func (s *Step) Out(channel, value string) {
	s.c.t.Helper()
	o := s.c.checkOutput(channel, true, false)
	s.actual[channel] = &outVal{text: value, raw: o.Raw}
}

// OutInt records an integer per-step output channel for this step.
func (s *Step) OutInt(channel string, value int) {
	s.c.t.Helper()
	s.c.checkOutput(channel, true, true)
	s.actual[channel] = &outVal{num: value}
}

// checkOutput validates a channel against the schema before the caller stores a
// value in it: that it is declared an Output at all, that it is scoped where the
// caller is setting it, and that the accessor matches its value kind.
func (c *Case) checkOutput(channel string, perStep, wantInt bool) Output {
	c.t.Helper()
	o := c.mustOutput(channel)
	if o.PerStep != perStep {
		if o.PerStep {
			c.fail("output %q is declared PerStep; set it on a Step, not the Case", channel)
		}
		c.fail("output %q is not declared PerStep; set it on the Case, not a Step", channel)
	}
	if o.Int != wantInt {
		if o.Int {
			c.fail("output %q is declared Int; use OutInt", channel)
		}
		c.fail("output %q is not declared Int; use Out", channel)
	}
	return o
}

func (c *Case) mustInput(name string) Input {
	c.t.Helper()
	if in, ok := c.sc.inputs[name]; ok {
		return in
	}
	c.fail("%q is not a declared Input; add it to Suite.Inputs%s", name, c.didYouMean(name, kindInput))
	return Input{}
}

func (c *Case) mustOutput(name string) Output {
	c.t.Helper()
	if o, ok := c.sc.outputs[name]; ok {
		return o
	}
	c.fail("%q is not a declared Output; add it to Suite.Outputs%s", name, c.didYouMean(name, kindOutput))
	return Output{}
}

// didYouMean explains a name the schema does not hold under the role the caller
// used. A name declared under a different role is the likeliest mistake and gets
// named outright; otherwise the declared names are listed, which for schemas
// this size beats guessing at the nearest one.
func (c *Case) didYouMean(name string, want kind) string {
	switch k, ok := c.sc.kinds[name]; {
	case !ok:
	case k == kindInput:
		return " (it is declared as an Input)"
	case k == kindAction:
		return " (it is declared as an Action)"
	default:
		return " (it is declared as an Output)"
	}
	declared := c.sc.names(want)
	if len(declared) == 0 {
		return ""
	}
	return fmt.Sprintf("; the suite declares %s", strings.Join(declared, ", "))
}

// fail reports a suite-authoring mistake against the line that made it.
func (c *Case) fail(format string, args ...any) {
	c.t.Helper()
	c.t.Fatalf("%s: case %q: %s", c.file, c.title, fmt.Sprintf(format, args...))
}

func newCase(sc *schema, file string) *Case {
	return &Case{
		sc:       sc,
		file:     file,
		t:        noTest{},
		inputs:   map[string]*inputVal{},
		expected: map[string]*outVal{},
		actual:   map[string]*outVal{},
		lines:    map[string]int{},
	}
}

func (c *Case) newStep(h header, body string) *Step {
	s := &Step{
		Name:     h.name,
		Args:     h.args,
		Body:     body,
		raw:      h.raw,
		line:     h.line,
		idx:      len(c.steps),
		c:        c,
		expected: map[string]*outVal{},
		actual:   map[string]*outVal{},
		lines:    map[string]int{},
	}
	c.steps = append(c.steps, s)
	return s
}

// label describes a step for a mismatch message, e.g. `step 2 (HOVER 1:6)`.
func (s *Step) label() string {
	var b strings.Builder
	fmt.Fprintf(&b, "step %d (%s", s.idx+1, s.Name)
	for _, a := range s.Args {
		b.WriteByte(' ')
		b.WriteString(a)
	}
	b.WriteByte(')')
	return b.String()
}

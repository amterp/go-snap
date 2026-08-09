package snap

import (
	"fmt"
	"strings"
	"testing"
)

// Case is one snapshot case: its hand-authored inputs and steps, and the
// output channels the runner sets.
type Case struct {
	sc      *schema
	markers Markers

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
	t         *testing.T
	ran       bool
	dirty     bool
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
	c.mustInput(section)
	_, ok := c.inputs[section]
	return ok
}

// Text returns an input section's body, or "" if absent.
func (c *Case) Text(section string) string {
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
	o := c.mustOutput(channel)
	if o.PerStep {
		c.fail("output %q is declared PerStep; set it on a Step, not the Case", channel)
	}
	if o.Int {
		c.fail("output %q is declared Int; use OutInt", channel)
	}
	c.actual[channel] = &outVal{text: value, raw: o.Raw}
}

// OutInt records an integer output channel.
func (c *Case) OutInt(channel string, value int) {
	o := c.mustOutput(channel)
	if o.PerStep {
		c.fail("output %q is declared PerStep; set it on a Step, not the Case", channel)
	}
	if !o.Int {
		c.fail("output %q is not declared Int; use Out", channel)
	}
	c.actual[channel] = &outVal{num: value}
}

// Out records a per-step output channel for this step.
func (s *Step) Out(channel, value string) {
	o := s.c.mustOutput(channel)
	if !o.PerStep {
		s.c.fail("output %q is not declared PerStep; set it on the Case, not a Step", channel)
	}
	if o.Int {
		s.c.fail("output %q is declared Int; use OutInt", channel)
	}
	s.actual[channel] = &outVal{text: value, raw: o.Raw}
}

// OutInt records an integer per-step output channel for this step.
func (s *Step) OutInt(channel string, value int) {
	o := s.c.mustOutput(channel)
	if !o.PerStep {
		s.c.fail("output %q is not declared PerStep; set it on the Case, not a Step", channel)
	}
	if !o.Int {
		s.c.fail("output %q is not declared Int; use Out", channel)
	}
	s.actual[channel] = &outVal{num: value}
}

func (c *Case) mustInput(name string) Input {
	if in, ok := c.sc.inputs[name]; ok {
		return in
	}
	c.fail("%q is not a declared Input; add it to Suite.Inputs%s", name, c.didYouMean(name))
	return Input{}
}

func (c *Case) mustOutput(name string) Output {
	if o, ok := c.sc.outputs[name]; ok {
		return o
	}
	c.fail("%q is not a declared Output; add it to Suite.Outputs%s", name, c.didYouMean(name))
	return Output{}
}

// didYouMean names the schema role a section was actually declared under, the
// likeliest cause of an accessor mismatch.
func (c *Case) didYouMean(name string) string {
	k, ok := c.sc.kinds[name]
	if !ok {
		return ""
	}
	switch k {
	case kindInput:
		return " (it is declared as an Input)"
	case kindAction:
		return " (it is declared as an Action)"
	default:
		return " (it is declared as an Output)"
	}
}

// fail reports a suite-authoring mistake. During a run that is a test failure;
// outside one (tooling calling the accessors) there is no test to fail, so it
// panics rather than returning a wrong answer.
func (c *Case) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if c.t != nil {
		c.t.Helper()
		c.t.Fatalf("%s: case %q: %s", c.file, c.title, msg)
	}
	panic(fmt.Sprintf("snap: %s: case %q: %s", c.file, c.title, msg))
}

func newCase(sc *schema, m Markers, file string) *Case {
	return &Case{
		sc:       sc,
		markers:  m,
		file:     file,
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

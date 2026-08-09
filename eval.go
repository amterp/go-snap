package snap

import "strings"

// mismatchKind distinguishes how a mismatch should be reported.
type mismatchKind int

const (
	mmText  mismatchKind = iota // rendered as a diff
	mmInt                       // rendered as want/got
	mmStale                     // the file asserts a channel the runner never set
)

// mismatch is one difference between a case's file and what the runner
// produced. It carries everything needed to report or to accept it.
type mismatch struct {
	channel string
	step    *Step // nil for case-scoped channels
	kind    mismatchKind
	want    string
	got     string
	wantNum int
	gotNum  int
	raw     bool
	line    int // header line of the offending section, 0 when it is absent
}

// eval compares what the runner set against what the file expects. It touches
// no filesystem and no *testing.T: the whole comparison story is a pure
// function of (schema, case), which is what makes it directly testable.
func eval(sc *schema, c *Case) []mismatch {
	var out []mismatch
	for _, name := range sc.outputOrder {
		o := sc.outputs[name]
		if o.PerStep {
			continue
		}
		out = appendMismatch(out, o, name, nil, c.expected, c.actual, c.lines, c.titleLine)
	}
	for _, st := range c.steps {
		for _, name := range sc.outputOrder {
			o := sc.outputs[name]
			if !o.PerStep {
				continue
			}
			out = appendMismatch(out, o, name, st, st.expected, st.actual, st.lines, st.line)
		}
	}
	return out
}

func appendMismatch(
	out []mismatch,
	o Output,
	name string,
	st *Step,
	expected, actual map[string]*outVal,
	lines map[string]int,
	fallbackLine int,
) []mismatch {
	exp, hasExp := expected[name]
	act, hasAct := actual[name]

	line := lines[name]
	if line == 0 {
		line = fallbackLine
	}

	if !hasAct {
		// A section nobody produces is an expectation nothing verifies - a
		// renamed channel or a runner that quietly stopped reporting.
		if hasExp {
			out = append(out, mismatch{channel: name, step: st, kind: mmStale, line: line})
		}
		return out
	}
	if !hasExp {
		exp = &outVal{raw: o.Raw}
	}

	if o.Int {
		if exp.num != act.num {
			out = append(out, mismatch{
				channel: name, step: st, kind: mmInt,
				wantNum: exp.num, gotNum: act.num, line: line,
			})
		}
		return out
	}

	raw := o.Raw || exp.raw
	want, got := exp.text, act.text
	if !raw {
		// Whether a stream ends in a newline is not something a verbatim text
		// file can pin down, so it is not something a text channel asserts.
		// Raw is how a suite says it cares.
		want, got = strings.TrimRight(want, "\n"), strings.TrimRight(got, "\n")
	}
	if want != got {
		out = append(out, mismatch{
			channel: name, step: st, kind: mmText,
			want: want, got: got, raw: raw, line: line,
		})
	}
	return out
}

// apply rewrites a case's expectations from what the runner produced. Only
// machine channels move; hand-authored sections are untouched.
func apply(sc *schema, c *Case) {
	if !c.ran {
		return
	}
	for _, name := range sc.outputOrder {
		o := sc.outputs[name]
		if o.PerStep {
			continue
		}
		applyChannel(o, name, c.expected, c.actual)
	}
	for _, st := range c.steps {
		for _, name := range sc.outputOrder {
			o := sc.outputs[name]
			if !o.PerStep {
				continue
			}
			applyChannel(o, name, st.expected, st.actual)
		}
	}
}

func applyChannel(o Output, name string, expected, actual map[string]*outVal) {
	act, ok := actual[name]
	if !ok {
		delete(expected, name)
		return
	}
	v := &outVal{num: act.num, text: act.text, raw: o.Raw || act.raw}
	if !v.raw {
		v.text = strings.TrimRight(v.text, "\n")
	}
	if isDefault(o, v) {
		delete(expected, name)
		return
	}
	expected[name] = v
}

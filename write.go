package snap

import (
	"os"
	"strconv"
	"strings"
)

// WriteFile renders cases in canonical form and writes them to path.
//
// The path is a parameter rather than a property of the cases so a caller can
// write somewhere other than where it read - the seam a pending-snapshot
// workflow would need.
func WriteFile(s *Suite, path string, cases []*Case) error {
	sc, err := s.schema()
	if err != nil {
		return err
	}
	return os.WriteFile(path, render(sc, s.Markers, cases), 0o644)
}

// render lays out cases canonically: meta, then inputs in declaration order,
// then steps in file order with each step's outputs beneath it, then the
// case-scoped outputs. Hand-authored content is reproduced exactly; only its
// placement is canonical.
func render(sc *schema, m Markers, cases []*Case) []byte {
	var b strings.Builder
	for i, c := range cases {
		if i > 0 {
			b.WriteString("\n")
		}
		// Meta bodies are escaped like any other: a title is ordinary text, and
		// nothing stops it from containing something header-shaped.
		writeSection(&b, m, secTitle, nil, false, escapeBody(m, c.title))
		if c.description != "" {
			writeSection(&b, m, secDescription, nil, false, escapeBody(m, c.description))
		}
		if c.skip != nil {
			writeSection(&b, m, secSkip, nil, false, escapeBody(m, *c.skip))
		}
		for _, name := range sc.inputOrder {
			v, ok := c.inputs[name]
			if !ok {
				continue
			}
			in := sc.inputs[name]
			switch {
			case in.List:
				writeSection(&b, m, name, nil, false, listBody(v.items, in.Quoted))
			case v.raw:
				writeSection(&b, m, name, nil, !in.Raw, strconv.Quote(v.text))
			default:
				writeSection(&b, m, name, nil, false, escapeBody(m, v.text))
			}
		}
		for _, st := range c.steps {
			body := st.Body
			if st.raw {
				body = strconv.Quote(body)
			} else {
				body = escapeBody(m, body)
			}
			writeSection(&b, m, st.Name, st.Args, st.raw, body)
			writeOutputs(&b, sc, m, st.expected, true)
		}
		writeOutputs(&b, sc, m, c.expected, false)
	}
	return []byte(b.String())
}

func writeOutputs(b *strings.Builder, sc *schema, m Markers, vals map[string]*outVal, perStep bool) {
	for _, name := range sc.outputOrder {
		o := sc.outputs[name]
		if o.PerStep != perStep {
			continue
		}
		v, ok := vals[name]
		if !ok || isDefault(o, v) {
			continue
		}
		switch {
		case o.Int:
			writeSection(b, m, name, nil, false, strconv.Itoa(v.num))
		case v.raw:
			writeSection(b, m, name, nil, !o.Raw, strconv.Quote(v.text))
		default:
			writeSection(b, m, name, nil, false, escapeBody(m, v.text))
		}
	}
}

// isDefault reports whether a channel's value carries no information, in which
// case the section is omitted: an absent output section already means "the
// channel's zero value".
func isDefault(o Output, v *outVal) bool {
	if o.Int {
		return v.num == 0
	}
	return v.text == ""
}

func writeSection(b *strings.Builder, m Markers, name string, args []string, raw bool, body string) {
	b.WriteString(renderHeader(m, name, args, raw))
	b.WriteString("\n")
	if body == "" {
		return
	}
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n")
}

func escapeBody(m Markers, s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = escapeLine(m, line)
	}
	return strings.Join(lines, "\n")
}

func listBody(items []string, quoted bool) string {
	if len(items) == 0 {
		return ""
	}
	out := make([]string, len(items))
	for i, item := range items {
		if quoted {
			out[i] = quoteListItem(item)
		} else {
			out[i] = item
		}
	}
	return strings.Join(out, "\n")
}

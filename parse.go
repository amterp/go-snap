package snap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseFile reads a .snap file against a suite's schema. It is exported for
// tooling - editors, migration scripts, orphan reports - that wants to read
// snapshot files without running them.
func ParseFile(s *Suite, path string) ([]*Case, error) {
	sc, err := s.schemaFor(false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(sc, s.Markers, path, data)
}

// section is one header plus the raw body lines beneath it.
type section struct {
	header
	body []string
}

func parse(sc *schema, m Markers, path string, data []byte) ([]*Case, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	lines := strings.Split(text, "\n")

	var sections []section
	cur := -1
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		lineNo := i + 1
		h, ok := parseHeader(m, line)
		if !ok {
			if cur < 0 {
				if strings.TrimSpace(line) != "" {
					return nil, fmt.Errorf("%s:%d: content before the first %s section", path, lineNo, secTitle)
				}
				continue
			}
			sections[cur].body = append(sections[cur].body, line)
			continue
		}
		h.line = lineNo
		if err := checkKnown(sc, m, path, h); err != nil {
			return nil, err
		}
		sections = append(sections, section{header: h})
		cur = len(sections) - 1
	}
	return assemble(sc, m, path, sections)
}

// checkKnown rejects a well-formed header naming a section the suite never
// declared. Silently treating it as body text is how a typo'd delimiter turns
// into expected output that asserts nothing.
func checkKnown(sc *schema, m Markers, path string, h header) error {
	if contains(reservedNames, h.name) {
		return nil
	}
	if _, ok := sc.kinds[h.name]; ok {
		return nil
	}
	return fmt.Errorf("%s:%d: unknown section %q; declare it in the Suite, or escape the line as %s",
		path, h.line, h.name, escapeLine(m, renderHeader(m, h.name, h.args, h.raw)))
}

func assemble(sc *schema, m Markers, path string, sections []section) ([]*Case, error) {
	var cases []*Case
	var c *Case
	titles := map[string]int{}

	errf := func(line int, format string, args ...any) error {
		return fmt.Errorf("%s:%d: %s", path, line, fmt.Sprintf(format, args...))
	}

	for _, s := range sections {
		if s.name == secTitle {
			if err := checkMetaHeader(s, errf); err != nil {
				return nil, err
			}
			title, err := scalar(s.body)
			if err != nil {
				return nil, errf(s.line, "%s: %v", secTitle, err)
			}
			if title == "" {
				return nil, errf(s.line, "%s is empty", secTitle)
			}
			if prev, dup := titles[title]; dup {
				return nil, errf(s.line, "duplicate title %q (first defined at line %d)", title, prev)
			}
			titles[title] = s.line
			c = newCase(sc, path)
			c.title = title
			c.titleLine = s.line
			cases = append(cases, c)
			continue
		}
		if c == nil {
			return nil, errf(s.line, "section %s before any %s", s.name, secTitle)
		}
		if err := c.addSection(sc, m, s, errf); err != nil {
			return nil, err
		}
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("%s: no cases (a file needs at least one %s section)", path, secTitle)
	}
	return cases, nil
}

// checkMetaHeader rejects decoration on a library-owned section. TITLE,
// DESCRIPTION and SKIP carry one line of prose and nothing else, so an argument
// or a [raw] flag is a mistake - and a mistake the reader would otherwise never
// hear about, since both would simply be dropped.
func checkMetaHeader(s section, errf func(int, string, ...any) error) error {
	if len(s.args) > 0 {
		return errf(s.line, "%s takes no header arguments, got %q", s.name, strings.Join(s.args, " "))
	}
	if s.raw {
		return errf(s.line, "%s holds one line of text, so %s is meaningless", s.name, rawFlag)
	}
	return nil
}

// checkRawFlag rejects a [raw] flag on a section that has no bytes for it to
// govern. Accepting it silently would let a suite believe it had asked for an
// exact comparison it is not getting.
func checkRawFlag(sc *schema, s section) string {
	if !s.raw {
		return ""
	}
	switch sc.kinds[s.name] {
	case kindAction:
		if !sc.actions[s.name].HasBody {
			return "takes no body"
		}
	case kindInput:
		if sc.inputs[s.name].List {
			return "is a List, whose items are quoted per line"
		}
	case kindOutput:
		if sc.outputs[s.name].Int {
			return "is an Int"
		}
	}
	return ""
}

func (c *Case) addSection(sc *schema, m Markers, s section, errf func(int, string, ...any) error) error {
	if why := checkRawFlag(sc, s); why != "" {
		return errf(s.line, "%s %s, so %s is meaningless", s.name, why, rawFlag)
	}

	switch s.name {
	case secDescription, secSkip:
		if err := checkMetaHeader(s, errf); err != nil {
			return err
		}
		if len(c.inputs) > 0 || len(c.steps) > 0 {
			return errf(s.line, "%s must come before the case's input and action sections", s.name)
		}
		if _, dup := c.lines[s.name]; dup {
			return errf(s.line, "%s appears twice in one case", s.name)
		}
		body, err := blockOf(s, false)
		if err != nil {
			return errf(s.line, "%s: %v", s.name, err)
		}
		if s.name == secDescription {
			c.description = body
		} else {
			c.skip = &body
		}
		c.lines[s.name] = s.line
		return nil
	}

	switch sc.kinds[s.name] {
	case kindInput:
		in := sc.inputs[s.name]
		if len(s.args) > 0 {
			return errf(s.line, "input %s takes no header arguments", s.name)
		}
		if len(c.steps) > 0 {
			return errf(s.line, "input %s must come before the case's action sections", s.name)
		}
		if _, dup := c.inputs[s.name]; dup {
			return errf(s.line, "input %s appears twice in one case", s.name)
		}
		v := &inputVal{raw: in.Raw || s.raw}
		if in.List {
			v.items = listOf(s.body, in.Quoted)
		} else {
			body, err := blockOf(s, v.raw)
			if err != nil {
				return errf(s.line, "input %s: %v", s.name, err)
			}
			v.text = body
		}
		c.inputs[s.name] = v
		c.lines[s.name] = s.line
		return nil

	case kindAction:
		a := sc.actions[s.name]
		if a.Args >= 0 && len(s.args) != a.Args {
			return errf(s.line, "action %s takes %d header argument(s), got %d", s.name, a.Args, len(s.args))
		}
		body, err := blockOf(s, s.raw)
		if err != nil {
			return errf(s.line, "action %s: %v", s.name, err)
		}
		if !a.HasBody && strings.TrimSpace(body) != "" {
			return errf(s.line, "action %s takes no body", s.name)
		}
		c.newStep(s.header, body)
		return nil

	case kindOutput:
		o := sc.outputs[s.name]
		if len(s.args) > 0 {
			return errf(s.line, "output %s takes no header arguments", s.name)
		}
		v := &outVal{raw: o.Raw || s.raw}
		if o.Int {
			body, err := scalar(s.body)
			if err != nil {
				return errf(s.line, "output %s: %v", s.name, err)
			}
			n, err := strconv.Atoi(body)
			if err != nil {
				return errf(s.line, "output %s is declared Int but holds %q", s.name, body)
			}
			v.num = n
		} else {
			body, err := blockOf(s, v.raw)
			if err != nil {
				return errf(s.line, "output %s: %v", s.name, err)
			}
			v.text = body
		}
		target, lines := c.expected, c.lines
		if o.PerStep {
			if len(c.steps) == 0 {
				return errf(s.line, "output %s is declared PerStep but no action precedes it", s.name)
			}
			last := c.steps[len(c.steps)-1]
			target, lines = last.expected, last.lines
		}
		if _, dup := target[s.name]; dup {
			return errf(s.line, "output %s appears twice for the same %s", s.name, ifElse(o.PerStep, "step", "case"))
		}
		target[s.name] = v
		lines[s.name] = s.line
		return nil
	}
	return errf(s.line, "unhandled section %s", s.name)
}

// blockOf decodes a body: a single Go-quoted line for raw sections, otherwise
// verbatim lines with per-line escapes resolved and the trailing blank run -
// which is the file's case separator, not content - removed.
func blockOf(s section, raw bool) (string, error) {
	if raw {
		return rawBody(s.body)
	}
	out := make([]string, len(s.body))
	for i, line := range s.body {
		out[i] = unescapeLine(line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n"), nil
}

func rawBody(body []string) (string, error) {
	var kept []string
	for _, line := range body {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	if len(kept) != 1 {
		return "", fmt.Errorf("a %s section holds exactly one Go-quoted line, got %d", rawFlag, len(kept))
	}
	s, err := strconv.Unquote(strings.TrimSpace(kept[0]))
	if err != nil {
		return "", fmt.Errorf("a %s section holds a Go-quoted string: %v", rawFlag, err)
	}
	return s, nil
}

// scalar decodes a one-line body such as TITLE or an Int output.
func scalar(body []string) (string, error) {
	var kept []string
	for _, line := range body {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, strings.TrimSpace(unescapeLine(line)))
		}
	}
	switch len(kept) {
	case 0:
		return "", nil
	case 1:
		return kept[0], nil
	default:
		return "", fmt.Errorf("expected a single line, got %d", len(kept))
	}
}

// listOf reads a list body as one item per non-blank line.
//
// Deliberately no unescapeLine here. A list line carries at most one convention:
// the Go-quoted form for a Quoted list, and nothing at all otherwise. Running the
// body-text escape as well would mean two decoders on the read side against one
// encoder on the write side, and the pairs that only one of them handles - an
// item that opens with the escape prefix, say - come back shorter than they went
// in. A non-Quoted list is plain text, exactly as it looks.
func listOf(body []string, quoted bool) []string {
	var items []string
	for _, line := range body {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if quoted {
			line = unquoteListItem(line)
		}
		items = append(items, line)
	}
	return items
}

func ifElse(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

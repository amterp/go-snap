package snap

import "strings"

// header is one parsed section header line.
type header struct {
	name string
	args []string
	raw  bool // the trailing [raw] flag was present
	line int
}

// parseHeader reports whether line is a section header under these markers,
// and if so its name, arguments and raw flag.
//
// A line that merely resembles a header but whose name breaks the name grammar
// (say "### hello ###") is content, not a malformed header: the grammar is what
// keeps ordinary prose from being mistaken for structure. A well-formed header
// naming an unregistered section is a different matter and fails loudly at the
// call site - that is what stops a typo'd delimiter from silently becoming
// expected output.
func parseHeader(m Markers, line string) (header, bool) {
	prefix, suffix := m.prefix(), m.suffix()
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return header{}, false
	}
	mid := line[len(prefix) : len(line)-len(suffix)]
	if len(prefix)+len(suffix) > len(line) {
		return header{}, false
	}
	fields := strings.Fields(mid)
	if len(fields) == 0 || !nameRe.MatchString(fields[0]) {
		return header{}, false
	}
	h := header{name: fields[0], args: fields[1:]}
	if n := len(h.args); n > 0 && h.args[n-1] == rawFlag {
		h.raw = true
		h.args = h.args[:n-1]
	}
	if len(h.args) == 0 {
		h.args = nil
	}
	return h, true
}

// renderHeader is the inverse of parseHeader.
func renderHeader(m Markers, name string, args []string, raw bool) string {
	var b strings.Builder
	b.WriteString(m.prefix())
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(a)
	}
	if raw {
		b.WriteByte(' ')
		b.WriteString(rawFlag)
	}
	b.WriteString(m.suffix())
	return b.String()
}

// looksLikeHeader reports whether a body line would be re-read as a header,
// and therefore has to be escaped when written.
func looksLikeHeader(m Markers, line string) bool {
	_, ok := parseHeader(m, line)
	return ok
}

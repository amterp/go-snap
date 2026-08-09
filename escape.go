package snap

import (
	"strconv"
	"strings"
)

// Bodies are stored verbatim, because verbatim is what makes these files worth
// reviewing. A few lines cannot survive that round trip - one that would re-read
// as a header, one an editor would strip trailing whitespace from, one carrying
// a carriage return - so the writer escapes exactly those, as a backslash
// followed by a Go-quoted string:
//
//	\"### TITLE ###"
//	\"trailing spaces   "
//
// A section that needs byte-exactness throughout should use Raw instead; this
// is the escape hatch for the occasional line, not a storage strategy.
const escapePrefix = `\"`

// escapeLine renders one body line, escaping it if it could not otherwise be
// read back unchanged.
func escapeLine(m Markers, line string) string {
	if needsEscape(m, line) {
		return `\` + strconv.Quote(line)
	}
	return line
}

// unescapeLine reverses escapeLine. A line that opens like an escape but does
// not parse as one is left alone rather than rejected: content is content, and
// the writer will quote it properly on the next rewrite.
func unescapeLine(line string) string {
	if !strings.HasPrefix(line, escapePrefix) {
		return line
	}
	if s, err := strconv.Unquote(line[1:]); err == nil {
		return s
	}
	return line
}

func needsEscape(m Markers, line string) bool {
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, escapePrefix) || looksLikeHeader(m, line) {
		return true
	}
	if trimmed := strings.TrimRight(line, " \t"); trimmed != line {
		return true
	}
	for _, r := range line {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return true
		}
	}
	return false
}

// quoteListItem renders a list item, quoting it when the plain form would not
// survive the line-per-item encoding - an empty item, or one whose meaning
// lives in its surrounding whitespace.
func quoteListItem(item string) string {
	if item == "" || strings.TrimSpace(item) != item || strings.HasPrefix(item, `"`) {
		return strconv.Quote(item)
	}
	return item
}

// unquoteListItem reverses quoteListItem for a Quoted list section.
func unquoteListItem(item string) string {
	if len(item) >= 2 && strings.HasPrefix(item, `"`) && strings.HasSuffix(item, `"`) {
		if s, err := strconv.Unquote(item); err == nil {
			return s
		}
	}
	return item
}

package example

import (
	"fmt"
	"sort"
	"strings"
)

// store is the toy stateful system behind the session suite. What matters for
// the example is that a meaningful test is a *sequence* against one instance,
// not a single call.
type store struct {
	data map[string]string
	log  []string
}

func newStore() *store { return &store{data: map[string]string{}} }

func (s *store) set(key, value string) string {
	prev, existed := s.data[key]
	s.data[key] = value
	s.log = append(s.log, "set "+key)
	if existed {
		return fmt.Sprintf("replaced %q", prev)
	}
	return "created"
}

func (s *store) get(key string) string {
	if v, ok := s.data[key]; ok {
		return v
	}
	return "<missing>"
}

func (s *store) del(key string) string {
	if _, ok := s.data[key]; !ok {
		return "no such key"
	}
	delete(s.data, key)
	s.log = append(s.log, "del "+key)
	return "deleted"
}

// dump renders the store deterministically. Sorting here rather than scrubbing
// later is the whole determinism story: Go map order would otherwise make this
// snapshot flap for reasons that have nothing to do with the code under test.
func (s *store) dump() string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, s.data[k])
	}
	if b.Len() == 0 {
		return "(empty)"
	}
	return b.String()
}

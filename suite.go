package snap

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// RunFunc drives the system under test for one case. It reads the case's
// hand-authored inputs and steps, and reports results by setting output
// channels. It never sees expected values: comparison is the engine's job, so
// a runner cannot accidentally become the assertion.
type RunFunc func(t *testing.T, c *Case)

// Suite declares a snapshot suite: its section schema, its runner, and its
// options. A schema is a table, so declare it as a struct literal.
//
// The library owns TITLE, DESCRIPTION and SKIP. Everything else is yours:
// Inputs are unique hand-authored sections, Actions are repeatable ordered
// ones forming a case's step sequence, and Outputs are machine-generated
// channels regenerated under -update.
type Suite struct {
	Run RunFunc

	Inputs  []Input
	Actions []Action
	Outputs []Output

	// Markers sets the section header syntax. The zero value is "### NAME ###",
	// chosen to avoid collision with most host-language syntax; override when
	// the content you snapshot would fight it.
	Markers Markers

	// Parallel runs each case with t.Parallel. Opt in only when the runner
	// holds no cross-case global state - the engine's own bookkeeping is
	// parallel-safe either way.
	Parallel bool

	Diff DiffConfig
}

// Input is a unique hand-authored section carrying a body.
type Input struct {
	Name string

	// List parses the body as one item per non-blank line, whitespace trimmed.
	List bool
	// Quoted lets a List item be written as a Go-quoted string, which is the
	// only way to express an empty or whitespace-significant item.
	Quoted bool
	// Raw stores the body as a single Go-quoted string, preserving bytes
	// exactly - trailing whitespace, CRs and all.
	Raw bool
}

// Action is a repeatable hand-authored section. Each occurrence in a case is
// one step, in file order, and may carry inline header arguments:
//
//	### HOVER 2:5 ###
//
// Argument syntax beyond tokenization belongs to the suite: the engine splits
// and counts tokens, the runner interprets them.
type Action struct {
	Name string

	// Args is the exact number of header argument tokens, or -1 for variadic.
	Args int
	// HasBody allows (and requires) a body under the header.
	HasBody bool
}

// Output is a machine-generated channel. The runner sets it; -update
// regenerates it from what the runner set.
type Output struct {
	Name string

	// PerStep correlates the channel with a step rather than the case, and
	// writes it directly under that step's header.
	PerStep bool
	// Int stores an integer; absent means zero.
	Int bool
	// Raw compares bytes exactly rather than forgiving trailing newlines, and
	// stores the value as a single Go-quoted string.
	Raw bool
}

// Markers is the section header syntax. A header line is
// Prefix + NAME + (" " + arg)... + Suffix, alone on its line.
type Markers struct {
	Prefix string // default "### "
	Suffix string // default " ###"
}

const (
	defaultPrefix = "### "
	defaultSuffix = " ###"

	// rawFlag as the final header token switches that one section instance to
	// byte-exact storage, as radfmt's [raw] title marker did before it.
	rawFlag = "[raw]"
)

func (m Markers) prefix() string {
	if m.Prefix == "" && m.Suffix == "" {
		return defaultPrefix
	}
	return m.Prefix
}

func (m Markers) suffix() string {
	if m.Prefix == "" && m.Suffix == "" {
		return defaultSuffix
	}
	return m.Suffix
}

// Library-owned section names. A suite cannot redeclare these.
const (
	secTitle       = "TITLE"
	secDescription = "DESCRIPTION"
	secSkip        = "SKIP"
)

var reservedNames = []string{secTitle, secDescription, secSkip}

var nameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// kind distinguishes the three schema roles a section name can hold.
type kind int

const (
	kindInput kind = iota
	kindAction
	kindOutput
)

// schema is the validated, indexed form of a Suite's declarations.
type schema struct {
	inputs  map[string]Input
	actions map[string]Action
	outputs map[string]Output
	kinds   map[string]kind

	// Registration order drives canonical write order, so a rewritten file
	// reads the way the suite was declared.
	inputOrder  []string
	outputOrder []string
}

func (s *Suite) schema() (*schema, error) {
	sc := &schema{
		inputs:  map[string]Input{},
		actions: map[string]Action{},
		outputs: map[string]Output{},
		kinds:   map[string]kind{},
	}
	var problems []string
	seen := map[string]bool{}

	declare := func(name string, k kind) bool {
		switch {
		case !nameRe.MatchString(name):
			problems = append(problems, fmt.Sprintf("section name %q must match [A-Z][A-Z0-9_]*", name))
			return false
		case contains(reservedNames, name):
			problems = append(problems, fmt.Sprintf("section name %q is reserved by go-snap", name))
			return false
		case seen[name]:
			problems = append(problems, fmt.Sprintf("section name %q declared more than once", name))
			return false
		}
		seen[name] = true
		sc.kinds[name] = k
		return true
	}

	for _, in := range s.Inputs {
		if !declare(in.Name, kindInput) {
			continue
		}
		if in.Quoted && !in.List {
			problems = append(problems, fmt.Sprintf("input %q sets Quoted without List", in.Name))
		}
		if in.Raw && in.List {
			problems = append(problems, fmt.Sprintf("input %q sets both Raw and List", in.Name))
		}
		sc.inputs[in.Name] = in
		sc.inputOrder = append(sc.inputOrder, in.Name)
	}
	for _, a := range s.Actions {
		if !declare(a.Name, kindAction) {
			continue
		}
		if a.Args < -1 {
			problems = append(problems, fmt.Sprintf("action %q has Args %d; use -1 for variadic", a.Name, a.Args))
		}
		sc.actions[a.Name] = a
	}
	for _, o := range s.Outputs {
		if !declare(o.Name, kindOutput) {
			continue
		}
		if o.Int && o.Raw {
			problems = append(problems, fmt.Sprintf("output %q sets both Int and Raw", o.Name))
		}
		sc.outputs[o.Name] = o
		sc.outputOrder = append(sc.outputOrder, o.Name)
	}

	if s.Run == nil {
		problems = append(problems, "Suite.Run is nil")
	}
	if len(sc.outputs) == 0 {
		problems = append(problems, "suite declares no Outputs, so no case can assert anything")
	}
	if p, sfx := s.Markers.prefix(), s.Markers.suffix(); p == "" {
		problems = append(problems, "Markers.Prefix is empty, which would make every line a section header")
	} else if strings.TrimSpace(p) == "" && strings.TrimSpace(sfx) == "" {
		problems = append(problems, "Markers.Prefix and Suffix are both blank, which would make every line a section header")
	}

	if len(problems) > 0 {
		return nil, errors.New("invalid snap.Suite:\n  - " + strings.Join(problems, "\n  - "))
	}
	return sc, nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

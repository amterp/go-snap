package snap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Updating snapshots is opt-in and targeted by default. A bare "rewrite
// everything" is how a real regression gets absorbed into the expectations
// that were supposed to catch it, so -update takes path substrings and rewrites
// only the files that match - while a mismatch anywhere else still fails the
// run.
//
//	go test ./... -update=errors/validation
//	go test ./... -update=types,errors
//	go test ./... -update-all
//
// SNAP_UPDATE is the same control as an environment variable, for the common
// case where the flag cannot reach the binary: `go test ./...` across packages
// that do not all import go-snap would reject an unknown -update flag.
//
//	SNAP_UPDATE=errors/validation go test ./...
//	SNAP_UPDATE=all go test ./...
const (
	flagUpdate    = "update"
	flagUpdateAll = "update-all"
	envUpdate     = "SNAP_UPDATE"
	envUpdateCI   = "SNAP_UPDATE_CI"
	envCI         = "CI"
)

// Flags are registered here rather than by each suite: flag registration is
// package-global state, so leaving it to callers means either every suite
// redeclares the same two flags or none of them can share one binary.
func init() {
	if flag.Lookup(flagUpdate) == nil {
		flag.String(flagUpdate, "",
			"comma-separated snapshot path substrings to rewrite; mismatches elsewhere still fail")
	}
	if flag.Lookup(flagUpdateAll) == nil {
		flag.Bool(flagUpdateAll, false,
			"rewrite ALL snapshot outputs - prefer -update=<path-substr>")
	}
}

type updateSpec struct {
	targets []string
	all     bool
	source  string
}

func (u updateSpec) active() bool { return u.all || len(u.targets) > 0 }

// matches reports whether a file is in scope for rewriting. Paths are
// slash-normalized first so the same -update=a/b works on Windows.
func (u updateSpec) matches(path string) bool {
	if u.all {
		return true
	}
	p := filepath.ToSlash(path)
	for _, t := range u.targets {
		if strings.Contains(p, t) {
			return true
		}
	}
	return false
}

func resolveUpdate() updateSpec {
	if lookupBool(flagUpdateAll) {
		return updateSpec{all: true, source: "-" + flagUpdateAll}
	}
	if v := lookupString(flagUpdate); v != "" {
		return updateSpec{targets: splitTargets(v), source: "-" + flagUpdate}
	}
	if v := strings.TrimSpace(os.Getenv(envUpdate)); v != "" {
		if v == "all" {
			return updateSpec{all: true, source: envUpdate + "=all"}
		}
		return updateSpec{targets: splitTargets(v), source: envUpdate}
	}
	return updateSpec{}
}

// check catches the two ways an update request is usually a mistake.
func (u updateSpec) check() error {
	for _, target := range u.targets {
		// -update takes a value, so `-update -run TestFoo` makes the flag
		// parser swallow -run as that value and quietly rewrite everything
		// the run touches.
		if strings.HasPrefix(target, "-") {
			return fmt.Errorf("%s value %q looks like a flag - write -%s=<path-substr>, or -%s to rewrite everything",
				u.source, target, flagUpdate, flagUpdateAll)
		}
	}
	if !u.active() {
		return nil
	}
	// Accepting new output in CI defeats the point of running there: nobody
	// reviews the result, and a green build proves only that the code agrees
	// with itself.
	if os.Getenv(envCI) != "" && os.Getenv(envUpdateCI) == "" {
		return fmt.Errorf("%s requested while %s is set: snapshots are not updated in CI.\n"+
			"Update locally and commit the result, or set %s=1 if this job exists to propose updates.",
			u.source, envCI, envUpdateCI)
	}
	return nil
}

func splitTargets(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func lookupString(name string) string {
	if f := flag.Lookup(name); f != nil {
		return strings.TrimSpace(f.Value.String())
	}
	return ""
}

func lookupBool(name string) bool {
	if f := flag.Lookup(name); f != nil {
		return f.Value.String() == "true"
	}
	return false
}

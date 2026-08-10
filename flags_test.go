package snap

import (
	"flag"
	"strings"
	"testing"
)

func TestUpdateTargetsMatchSlashNormalizedPaths(t *testing.T) {
	u := updateSpec{targets: []string{"errors/validation"}}
	// filepath.Join produces backslashes on Windows; a target written with
	// forward slashes has to match there too.
	if !u.matches("snapshots/errors/validation/break.snap") {
		t.Error("expected a match on a forward-slash path")
	}
	if u.matches("snapshots/types/str.snap") {
		t.Error("unrelated file should not match")
	}
	if !(updateSpec{all: true}).matches("anything.snap") {
		t.Error("update-all should match everything")
	}
	if (updateSpec{}).matches("anything.snap") {
		t.Error("an inactive spec should match nothing")
	}
}

func TestUpdateFlagSwallowingIsCaught(t *testing.T) {
	// `go test -update -run TestFoo` makes the flag parser treat -run as the
	// value of -update, which would otherwise rewrite every file the run
	// touches.
	err := updateSpec{targets: []string{"-run"}, source: "-update"}.check()
	if err == nil {
		t.Fatal("expected an error for a flag-shaped update target")
	}
	if !strings.Contains(err.Error(), "looks like a flag") {
		t.Errorf("error = %v", err)
	}
}

func TestUpdateIsRefusedInCI(t *testing.T) {
	t.Setenv(envCI, "true")
	err := updateSpec{all: true, source: "-update-all"}.check()
	if err == nil {
		t.Fatal("expected updates to be refused while CI is set")
	}
	if !strings.Contains(err.Error(), "not updated in CI") {
		t.Errorf("error = %v", err)
	}

	t.Setenv(envUpdateCI, "1")
	if err := (updateSpec{all: true, source: "-update-all"}).check(); err != nil {
		t.Errorf("%s should permit a deliberate update job: %v", envUpdateCI, err)
	}
}

func TestUpdateIsAllowedOutsideCI(t *testing.T) {
	t.Setenv(envCI, "")
	if err := (updateSpec{targets: []string{"errors"}, source: "-update"}).check(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnvFallbackResolvesTargetsAndAll(t *testing.T) {
	t.Setenv(envUpdate, "errors, types ")
	got := resolveUpdate()
	if got.all || len(got.targets) != 2 || got.targets[0] != "errors" || got.targets[1] != "types" {
		t.Errorf("resolveUpdate = %+v", got)
	}

	t.Setenv(envUpdate, "all")
	if got := resolveUpdate(); !got.all {
		t.Errorf("SNAP_UPDATE=all should rewrite everything, got %+v", got)
	}

	t.Setenv(envUpdate, "")
	if got := resolveUpdate(); got.active() {
		t.Errorf("no flags and no env should mean no update, got %+v", got)
	}
}

func TestBoolUpdateFlagFromAnotherPackageIsRefused(t *testing.T) {
	// Several Go snapshot libraries spell -update as a bool. Adopting one would
	// read its value as the path substring "true", which matches almost nothing
	// and occasionally the wrong thing - silently, either way.
	saved := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = saved })
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	flag.Bool(flagUpdate, false, "some other library's flag")

	err := updateSpec{targets: []string{"errors"}, source: "-update"}.check()
	if err == nil {
		t.Fatal("expected a bool -update from another package to be refused")
	}
	if !strings.Contains(err.Error(), "boolean flag") {
		t.Errorf("error = %v", err)
	}
}

func TestStringUpdateFlagIsAdoptedQuietly(t *testing.T) {
	saved := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = saved })
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	flag.String(flagUpdate, "", "a compatible pre-existing flag")

	if err := (updateSpec{targets: []string{"errors"}, source: "-update"}).check(); err != nil {
		t.Errorf("a string -update is the shape go-snap needs: %v", err)
	}
}

// An empty -update= is an answer, not a shrug. A Makefile writing
// `go test ./... -update=$(TARGET)` with TARGET unset must mean "do not update",
// not "inherit whatever the developer happens to export".
func TestAnExplicitlyEmptyFlagBeatsTheEnvironment(t *testing.T) {
	saved := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = saved })
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	flag.String(flagUpdate, "", "")
	flag.Bool(flagUpdateAll, false, "")
	if err := flag.CommandLine.Parse([]string{"-" + flagUpdate + "="}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envUpdate, "all")

	if got := resolveUpdate(); got.active() {
		t.Errorf("a stray %s overrode an explicit empty target: %+v", envUpdate, got)
	}
}

func TestTheEnvironmentStillAppliesWhenNoFlagIsPassed(t *testing.T) {
	saved := flag.CommandLine
	t.Cleanup(func() { flag.CommandLine = saved })
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	flag.String(flagUpdate, "", "")
	flag.Bool(flagUpdateAll, false, "")
	t.Setenv(envUpdate, "errors")

	if got := resolveUpdate(); len(got.targets) != 1 || got.targets[0] != "errors" {
		t.Errorf("resolveUpdate = %+v", got)
	}
}

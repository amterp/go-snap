package snap

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Ext is the snapshot file extension. PendingExt is reserved: files carrying it
// are never discovered as suite files, leaving room for a review workflow that
// writes proposed output beside the accepted file rather than over it.
const (
	Ext        = ".snap"
	PendingExt = ".snap.new"
)

// warnFileSize is the point at which a snapshot file has stopped being
// reviewable. Nobody reads a 200KB diff, which means nobody is really
// reviewing those snapshots at all.
const warnFileSize = 256 << 10

// reporter is the slice of *testing.T the engine itself needs. Keeping it
// separate from the *testing.T handed to a suite's runner lets go-snap test
// its own failure output without a subprocess.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Skip(args ...any)
	// Failed reports whether this case has already been failed - by the
	// suite's own runner, since the engine reports nothing until afterwards.
	Failed() bool
}

// fileState is one snapshot file through a run: what was on disk, what parsed
// out of it, and whether this run may rewrite it.
type fileState struct {
	path     string
	orig     []byte
	cases    []*Case
	updating bool
}

// Run executes every .snap file under dir as subtests of t.
//
// Each case becomes a subtest named "<file>/<title>", so `-run` can select one
// file or one case. Under -update, a file is rewritten once all of its cases
// have finished.
func Run(t *testing.T, dir string, s *Suite) {
	t.Helper()

	sc, err := s.schema()
	if err != nil {
		t.Fatal(err)
	}
	upd := resolveUpdate()
	if err := upd.check(); err != nil {
		t.Fatal(err)
	}

	paths, err := discover(dir)
	switch {
	case os.IsNotExist(err):
		t.Fatalf("no snapshot directory at %s - Run's dir argument is relative to the test's package directory, so check the path or create the directory and add a %s file", dir, Ext)
	case err != nil:
		t.Fatalf("reading snapshots under %s: %v", dir, err)
	case len(paths) == 0:
		t.Fatalf("no %s files under %s - snapshot suites are declared by their files, so an empty directory is a misconfiguration", Ext, dir)
	}

	// Every unreadable file is reported together. Failing on the first one
	// means fixing it only to discover the next, which costs a whole run per
	// broken file - the same reason schema() collects its problems.
	states := make([]*fileState, 0, len(paths))
	var broken []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			broken = append(broken, fmt.Sprintf("reading %s: %v", path, err))
			continue
		}
		cases, err := parse(sc, s.Markers, path, data)
		if err != nil {
			broken = append(broken, err.Error())
			continue
		}
		states = append(states, &fileState{path: path, orig: data, cases: cases, updating: upd.matches(path)})
	}
	if len(broken) > 0 {
		t.Fatalf("%d of %d snapshot files could not be read:\n  - %s",
			len(broken), len(paths), strings.Join(broken, "\n  - "))
	}

	// Rewriting on cleanup is what makes Parallel safe: Go runs a test's
	// cleanups only after every subtest it spawned, parallel ones included,
	// has finished.
	t.Cleanup(func() {
		if !upd.active() {
			return
		}
		for _, st := range states {
			written, err := st.rewrite(sc, s.Markers)
			switch {
			case err != nil:
				t.Errorf("updating %s: %v", st.path, err)
			case written:
				t.Logf("updated %s", filepath.ToSlash(st.path))
				if size := len(st.orig); size > warnFileSize {
					t.Logf("%s is %dKB; consider splitting it or narrowing what its cases capture, "+
						"since a file this size stops getting reviewed", filepath.ToSlash(st.path), size>>10)
				}
			}
		}
	})

	for _, st := range states {
		rel := filepath.ToSlash(st.path)
		group := strings.TrimSuffix(rel, Ext)
		for _, c := range st.cases {
			c, updating := c, st.updating
			t.Run(group+"/"+c.Title(), func(t *testing.T) {
				if s.Parallel {
					t.Parallel()
				}
				runCase(t, t, s, sc, c, rel, updating)
			})
		}
	}
}

// rewrite regenerates the file from what the runner produced, and writes it
// only if the result differs from what is on disk.
//
// Comparing rendered bytes rather than tracking "did any case change" means an
// update also migrates a file whose content is unchanged but whose formatting
// predates a change to the format itself - the alternative is snapshots that
// can never be reformatted.
func (st *fileState) rewrite(sc *schema, m Markers) (bool, error) {
	if !st.updating {
		return false, nil
	}
	for _, c := range st.cases {
		apply(sc, c)
	}
	next := render(sc, m, st.cases)
	if bytes.Equal(next, st.orig) {
		return false, nil
	}
	if err := os.WriteFile(st.path, next, 0o644); err != nil {
		return false, err
	}
	st.orig = next
	return true, nil
}

func runCase(t *testing.T, rep reporter, s *Suite, sc *schema, c *Case, rel string, updating bool) {
	rep.Helper()
	c.t = t
	if c.skip != nil {
		reason := *c.skip
		if reason == "" {
			reason = "no reason given"
		}
		rep.Skip(reason)
		return
	}

	s.Run(t, c)
	// An update may only bless output from a runner that both finished and is
	// happy with what it produced. t.Fatal never reaches this line at all, but
	// the commoner `t.Errorf(...); return` does, and blessing a partial run
	// there deletes the very expectations the failure should have protected -
	// while the terminal says FAIL and gives no hint the file was touched.
	c.ran = !rep.Failed()

	mismatches := eval(sc, c)
	if updating {
		if !c.ran {
			rep.Errorf("not accepting output for this case: its runner reported a failure, so what it produced cannot be trusted.\n" +
				"The file keeps its current expectations. Fix the failure, then run the update again.")
		}
		return
	}
	for _, m := range mismatches {
		rep.Errorf("%s", s.Diff.report(m, rel))
	}
}

func discover(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, Ext) && !strings.HasSuffix(name, PendingExt) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

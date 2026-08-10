# go-snap

Snapshot testing for text in Go: CLI output, rendered documents, language-server
responses, interactive terminal sessions.

You declare what a suite's snapshot files may contain and write one function
that drives the system under test. go-snap discovers the files, runs a subtest
per case, compares, diffs, and rewrites expectations when you accept them.

```go
var suite = snap.Suite{
	Run:     runCase,
	Inputs:  []snap.Input{{Name: "ARGS", List: true, Quoted: true}},
	Outputs: []snap.Output{{Name: "STDOUT"}, {Name: "STDERR"}, {Name: "EXIT", Int: true}},
}

func TestSnapshots(t *testing.T) { snap.Run(t, "snapshots", &suite) }

func runCase(t *testing.T, c *snap.Case) {
	stdout, stderr, exit := myCLI(c.List("ARGS"))
	c.Out("STDOUT", stdout)
	c.Out("STDERR", stderr)
	c.OutInt("EXIT", exit)
}
```

That is the whole integration. Cases live in `snapshots/*.snap`:

```
### TITLE ###
greets a name
### ARGS ###
World
### STDOUT ###
Hello, World!

### TITLE ###
unknown flag names the flag
### ARGS ###
--yell
### STDERR ###
greet: unknown flag --yell
### EXIT ###
2
```

One file holds many cases, so related behavior is reviewed together. Adding a
case means adding text, not writing Go.

## Accepting new output

When output changes, the failure tells you where and how to accept it:

```
STDOUT mismatch:
1 │ - Hello, World! │ 1 │ + Howdy, World!

  at snapshots/cli/basic.snap:5
  to accept: -update=snapshots/cli/basic.snap
```

Updates are targeted. `-update=<substr>` rewrites only files whose path contains
one of the given substrings, and a mismatch in any other file still fails the
run. Accepting one change therefore cannot quietly absorb an unrelated
regression somewhere else - the failure mode that makes a blanket "update
everything" flag dangerous.

```sh
go test ./... -update=errors/validation     # accept one area
go test ./... -update=types,errors          # accept several
go test ./... -update-all                   # deliberate sweep
SNAP_UPDATE=errors go test ./...            # same, via the environment
```

`SNAP_UPDATE` exists for `go test ./...` across a repo where not every package
imports go-snap: an unknown `-update` flag would be rejected by the packages that
never registered it, while the environment variable reaches all of them.

Two guards apply. A value starting with `-` is refused, because `-update -run
TestFoo` makes Go's flag parser swallow `-run` as the update target and rewrite
whatever the run touches. And updates are refused when `CI` is set, since nobody
reviews the result there; set `SNAP_UPDATE_CI=1` for a job that exists to propose
updates.

An update also reformats. go-snap rewrites a file whenever its canonical
rendering differs from what is on disk, not only when output changed, so
`-update-all` doubles as a formatter and a format migration can be applied to
existing snapshots.

## Cases with steps

Some systems cannot be tested one call at a time. An `Action` is a section that
may repeat, carry arguments in its header, and produce its own output:

```go
var suite = snap.Suite{
	Run:     runSession,
	Actions: []snap.Action{{Name: "SET", Args: 2}, {Name: "GET", Args: 1}},
	Outputs: []snap.Output{{Name: "RESULT", PerStep: true}, {Name: "FINAL"}},
}

func runSession(t *testing.T, c *snap.Case) {
	s := newStore()
	for _, step := range c.Steps() {
		switch step.Name {
		case "SET":
			step.Out("RESULT", s.set(step.Args[0], step.Args[1]))
		case "GET":
			step.Out("RESULT", s.get(step.Args[0]))
		}
	}
	c.Out("FINAL", s.dump())
}
```

```
### TITLE ###
setting an existing key reports what it replaced
### SET k first ###
### RESULT ###
created
### SET k second ###
### RESULT ###
replaced "first"
### FINAL ###
k = second
```

A `PerStep` channel is written directly under the action that produced it, so a
reader never counts to find out which result belongs to which step.

The single-call suite is this same model with no actions declared. Nothing is
paid for the generality up front, and a suite that grows into sequences does not
have to change shape - the alternative is discovering mid-project that the
format cannot express what you need, which is how snapshot harnesses end up
forked.

Argument syntax belongs to the suite. go-snap splits header tokens and checks
how many there are; what `2:5` means is the runner's business.

## Files

A section header is `### NAME ###` on its own line, and everything until the
next header is its body. `### TITLE ###` starts a case; titles are unique within
a file and become the subtest name.

`### DESCRIPTION ###` records why a case exists. `### SKIP ###` skips it, with
the body as the reason.

**A header naming a section the suite never declared is an error**, not content.
A mistyped delimiter should fail loudly rather than become expected output that
asserts nothing.

**Absent and empty differ, and mean different things on each side.** For inputs,
`c.Has("STDIN")` distinguishes an empty section from a missing one. For outputs,
an absent section asserts the channel's zero value, so a case with no `STDERR`
section asserts that stderr is empty. A channel the runner never sets asserts
nothing at all - which is how a suite makes an assertion conditional. The
converse, a section in the file that the runner never sets, is an error: it is an
expectation nothing verifies.

Because absence carries that meaning on both sides, an update also removes a
section it can reproduce: write `### EXIT ###` with a body of `0` and the next
accepted update deletes it. The assertion is unchanged - an absent `EXIT` still
requires zero - but the line you typed will not survive.

**Bodies are stored verbatim**, which is what makes these files worth reviewing.
Trailing newlines are not compared, because a text file cannot pin them down
reliably. Three kinds of line cannot survive that round trip - one that would
re-read as a header, one an editor would strip trailing whitespace from, one
carrying a carriage return - and the writer escapes exactly those as a backslash
followed by a Go-quoted string:

```
### STDOUT ###
ordinary line
\"### TITLE ###"
\"trailing spaces   "
```

When a section is about bytes rather than lines, declare it `Raw` or mark one
instance `### STDOUT [raw] ###`. Its body is a single Go-quoted string, compared
exactly, and its diffs quote each line so invisible differences are visible.

A `List` input is one item per non-blank line, whitespace trimmed, and carries
no escape convention: a line means the characters in it. Add `Quoted` and an item
may be written as a Go-quoted string, which is the only way to express an empty
item, a space-padded one, or one holding a newline.

`Markers` changes the header syntax when `###` collides with the content you
snapshot:

```go
Markers: snap.Markers{Prefix: "=== ", Suffix: " ==="}
```

Choose them before the suite has files. Changing them later leaves every existing
snapshot unparseable, since its headers are written in the old syntax.

Section names are strings in two places - the `Suite` and the runner - and
nothing links them. For a suite big enough that the drift would go unnoticed,
keep the declaration in a variable and read the name off it:

```go
var KeysSection = snap.Input{Name: "KEYS", List: true}

keys := c.List(KeysSection.Name)
```

## Determinism

go-snap does not scrub timestamps, paths, or IDs, and offers no matcher syntax
for "any number here".

Inject the nondeterminism instead. A suite that passes a fixed clock and a
deterministic ID sequence gets stable snapshots by construction *and* keeps the
real values visible in the file, where a reviewer can see that the timestamp is
wrong. Scrubbing hides exactly the values a snapshot exists to pin. When it is
genuinely unavoidable - a temp directory in an error message - do it in the
runner before the value reaches a channel:

```go
c.Out("STDERR", strings.ReplaceAll(stderr, sandbox.Home, "$HOME"))
```

Fixtures follow the same principle: `### FIXTURE ###` is an ordinary input
section, and a `map[string]func()` in the runner is the entire feature.

## Interactive prompts

The `prompt` package snapshots terminal prompts. Scripted keystrokes go in,
rendered frames come out:

```
### KEYS ###
"su"
enter
### CHOICE ###
Sushi
### FRAMES ###
--- frame 0 (initial) ---
Pick a food
> Hamburger
  Pizza
  Sushi
--- frame 1 (s) ---
Pick a food
/s
> Sushi
```

`KEYS` is hand-authored; `FRAMES` regenerates under `-update` like any other
channel. Each frame is labeled with the keystroke that produced it, which is what
lets a reviewer tell a rendering bug from an input-handling bug.

Install `prompt.NoKeys{}` as the driver for cases that script nothing. It
fails immediately instead of falling through to a terminal that is not there,
where the suite would hang and look like broken infrastructure.

Prompts are driven by [radish](https://github.com/amterp/radish), which the
package's types name directly rather than wrapping. The core package does not
import it.

## Parallelism

Set `Parallel: true` and each case runs with `t.Parallel()`. File rewrites happen
in a cleanup, which Go runs only after every subtest has finished, so updating
and parallelism compose. Suites whose runner touches package-level state should
leave it off.

## What go-snap does not do

- **Determinism seams.** Clocks, IDs, and environment belong to your harness.
- **Running your code.** The runner produces strings however it likes; go-snap
  never spawns a process.
- **Obsolete-snapshot detection.** Every file found is run, so a snapshot cannot
  be orphaned by a deleted test. Deleting a case means deleting text.
- **Review tooling.** Updates are applied in place and reviewed as a git diff.
  `.snap.new` is reserved so a pending-snapshot workflow can be added later
  without changing the format.

## Reference

| `Suite` | |
|---|---|
| `Run` | `func(t *testing.T, c *snap.Case)` |
| `Inputs` | unique hand-authored sections: `{Name, List, Quoted, Raw}` |
| `Actions` | repeatable ordered sections: `{Name, Args, HasBody}`; `Args: -1` is variadic |
| `Outputs` | machine-generated channels: `{Name, PerStep, Int, Raw}` |
| `Markers` | header syntax; zero value is `### NAME ###` |
| `Parallel` | run cases with `t.Parallel()` |
| `Diff` | `{Width, Color, Func}`; honors `NO_COLOR` and `SNAP_NO_COLOR` |

`Case`: `Title`, `Description`, `Has`, `Text`, `List`, `Steps`, `Out`, `OutInt`,
`Expected`, `ExpectedInt`.
`Step`: `Name`, `Args`, `Body`, `Out`, `OutInt`, `Expected`, `ExpectedInt`.

`ParseFile` and `WriteFile` expose the format layer for tooling, and `Expected`
reads what a file records for a channel. A suite needs none of these - a runner
reports what happened and lets the engine compare - but a formatter, a linter, or
a review tool reading a pending file does.

`example/` is a working suite of each shape, and is run by CI.

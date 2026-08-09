// Package snap is a snapshot-testing library for text: CLI output, rendered
// documents, language-server responses, interactive terminal sessions.
//
// A suite is declared as data - which sections its files may contain, and which
// output channels its runner reports - plus one function that drives the system
// under test. The library owns everything else: discovering files, parsing them,
// running a subtest per case, comparing, diffing, and rewriting expectations
// when you accept them.
//
//	var suite = snap.Suite{
//		Run:     runCase,
//		Inputs:  []snap.Input{{Name: "ARGS", List: true, Quoted: true}},
//		Outputs: []snap.Output{{Name: "STDOUT"}, {Name: "EXIT", Int: true}},
//	}
//
//	func TestSnapshots(t *testing.T) { snap.Run(t, "snapshots", &suite) }
//
//	func runCase(t *testing.T, c *snap.Case) {
//		stdout, exit := runCLI(c.List("ARGS"))
//		c.Out("STDOUT", stdout)
//		c.OutInt("EXIT", exit)
//	}
//
// # Files
//
// One file holds many cases, so related behavior is reviewed together:
//
//	### TITLE ###
//	rejects an unknown flag
//	### ARGS ###
//	--nope
//	### STDERR ###
//	unknown flag: --nope
//	### EXIT ###
//	2
//
// Sections a suite never declared are an error, not content: a mistyped
// delimiter should fail loudly rather than quietly become expected output that
// asserts nothing.
//
// # Cases with steps
//
// An Action is a section that may repeat, carry header arguments, and produce
// its own output - enough to snapshot a session rather than a single call:
//
//	### HOVER 1:6 ###
//	### RESULT ###
//	`foo`: int
//	### RENAME 0:0 bar ###
//	### RESULT ###
//	{"changes": 2}
//
// The single-shot case above is the same model with no steps, so a suite can
// grow into sequences without changing shape.
//
// # Accepting new output
//
// Snapshot updates are targeted:
//
//	go test ./... -update=errors/validation
//
// Only matching files are rewritten; a mismatch anywhere else still fails, so
// accepting one change cannot quietly absorb an unrelated regression. Use
// -update-all for a deliberate sweep, or SNAP_UPDATE=<substr> when the flag
// cannot reach the test binary. Every failure prints the exact command that
// would accept it.
//
// # Determinism
//
// The library does not scrub timestamps, paths or IDs. Suites that inject a
// fixed clock and deterministic IDs get stable snapshots by construction, and
// keep the real values visible in the file; scrubbing is a last resort, applied
// in the runner before the value reaches a channel.
package snap

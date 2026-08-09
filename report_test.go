package snap

import (
	"strconv"
	"testing"
)

// go-snap's failure output is a deliverable, not a detail: it is what a
// developer - or an agent - reads to decide whether a change is a regression
// or an improvement. So it is pinned the way any other text surface would be,
// with go-snap.
var reportSuite = Suite{
	Run: runReport,
	Inputs: []Input{
		{Name: "KIND"},
		{Name: "CHANNEL"},
		{Name: "STEP"},
		{Name: "WANT"},
		{Name: "GOT"},
	},
	Outputs:  []Output{{Name: "REPORT"}},
	Parallel: true,
}

func TestReportSnapshots(t *testing.T) {
	Run(t, "snapshots/report", &reportSuite)
}

func runReport(t *testing.T, c *Case) {
	channel := c.Text("CHANNEL")
	if channel == "" {
		channel = "STDOUT"
	}

	m := mismatch{channel: channel, want: c.Text("WANT"), got: c.Text("GOT"), line: 12}

	switch kind := c.Text("KIND"); kind {
	case "", "text":
		m.kind = mmText
	case "raw":
		m.kind, m.raw = mmText, true
	case "int":
		m.kind = mmInt
		m.wantNum = mustAtoi(t, c.Text("WANT"))
		m.gotNum = mustAtoi(t, c.Text("GOT"))
	case "stale":
		m.kind = mmStale
	default:
		t.Fatalf("unknown KIND %q", kind)
	}

	if label := c.Text("STEP"); label != "" {
		m.step = &Step{Name: label, idx: 1}
	}

	off := false
	cfg := DiffConfig{Color: &off, Width: 80}
	c.Out("REPORT", cfg.report(m, "snapshots/example.snap"))
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("KIND int needs numeric WANT/GOT, got %q", s)
	}
	return n
}

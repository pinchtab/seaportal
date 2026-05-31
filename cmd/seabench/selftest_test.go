package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJSONL is a small test helper that writes one record per line so
// the parser sees exactly what record.sh would produce in the wild.
func writeJSONL(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// writeGroup writes a minimal group-selftest.md containing only the
// inline `**Expected escalation:** …` lines the parser cares about.
func writeGroup(t *testing.T, dir string, entries map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Group selftest\n\n")
	for id, v := range entries {
		b.WriteString("### " + id + " task description\n")
		b.WriteString("Some prose.\n")
		b.WriteString("**Expected escalation:** " + v + "\n\n")
	}
	p := filepath.Join(dir, "group-selftest.md")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write group: %v", err)
	}
	return p
}

// TestSelftest_ParseJSONLAndCompute feeds a synthetic transcript with a
// mix of pass / fail / escalate outcomes and asserts the headline
// numbers match a hand-calculation, including escalation correctness.
func TestSelftest_ParseJSONLAndCompute(t *testing.T) {
	dir := t.TempDir()
	jsonl := writeJSONL(t, dir, "run.jsonl", []string{
		`{"ts":"2026-05-17T10:00:00Z","step":"1.1","outcome":"pass","note":"got title"}`,
		`{"ts":"2026-05-17T10:00:01Z","step":"1.2","outcome":"pass","note":"ok"}`,
		`{"ts":"2026-05-17T10:00:02Z","step":"1.3","outcome":"fail","note":"missed"}`,
		`{"ts":"2026-05-17T10:00:03Z","step":"1.4","outcome":"escalate","note":"blocked"}`,
		`{"ts":"2026-05-17T10:00:04Z","step":"1.5","outcome":"escalate","note":"spa"}`,
		// Task 1.6 takes two ops before passing — exercises op-count averaging
		// and "final outcome = last step" behaviour.
		`{"ts":"2026-05-17T10:00:05Z","step":"1.6","outcome":"fail","note":"first try"}`,
		`{"ts":"2026-05-17T10:00:06Z","step":"1.6","outcome":"pass","note":"retry"}`,
	})

	expected := map[string]string{
		"1.1": "no",
		"1.2": "no",
		"1.3": "no",
		"1.4": "yes",
		"1.5": "no", // escalation expected NO but agent escalated → wrong
		"1.6": "no",
	}
	group := writeGroup(t, dir, expected)

	steps, err := parseSelftestJSONL(jsonl)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(steps) != 7 {
		t.Fatalf("expected 7 steps, got %d", len(steps))
	}

	exp, err := loadExpectedEscalations(group)
	if err != nil {
		t.Fatalf("load group: %v", err)
	}
	if exp["1.4"] != "yes" || exp["1.5"] != "no" {
		t.Fatalf("unexpected expected map: %#v", exp)
	}

	tasks := collapseSteps(steps, exp)
	if len(tasks) != 6 {
		t.Fatalf("expected 6 tasks, got %d", len(tasks))
	}

	m := computeSelftestMetrics(tasks)

	// Hand-calc:
	//  1.1 pass (no)        → passed
	//  1.2 pass (no)        → passed
	//  1.3 fail (no)        → not passed
	//  1.4 escalate (yes)   → passed, correct escalation
	//  1.5 escalate (no)    → not passed, wrong escalation
	//  1.6 pass (no)        → passed (2 ops)
	// passed=4 of 6 → 0.6667
	// total ops = 1+1+1+1+1+2 = 7 → avg 7/6 = 1.1667
	// escalations: 2 total, 1 correct → 0.5
	if m.TotalTasks != 6 || m.PassedTasks != 4 {
		t.Fatalf("counts: %+v", m)
	}
	if !selftestAlmostEqual(m.CompletionRate, 4.0/6.0) {
		t.Errorf("completion rate: got %v want %v", m.CompletionRate, 4.0/6.0)
	}
	if !selftestAlmostEqual(m.AvgOps, 7.0/6.0) {
		t.Errorf("avg ops: got %v want %v", m.AvgOps, 7.0/6.0)
	}
	if m.TotalEscalations != 2 || m.CorrectEscalations != 1 {
		t.Errorf("escalations: %+v", m)
	}
	if !m.EscalationApplicable || !selftestAlmostEqual(m.EscalationCorrectness, 0.5) {
		t.Errorf("escalation correctness: %+v", m)
	}
}

// TestSelftest_DiffsAgainstPrior writes a prior report.json then runs
// the diff helper against a synthetic current and asserts that
// regressed / recovered task ids are surfaced correctly along with
// deltas.
func TestSelftest_DiffsAgainstPrior(t *testing.T) {
	prior := SelftestReport{
		Metrics: SelftestMetrics{
			CompletionRate:        0.8,
			AvgOps:                2.0,
			EscalationCorrectness: 1.0,
		},
		Tasks: []SelftestTask{
			{ID: "1.1", Passed: true},
			{ID: "1.2", Passed: true},
			{ID: "1.3", Passed: false},
			{ID: "1.4", Passed: true},
		},
	}
	current := SelftestReport{
		Metrics: SelftestMetrics{
			CompletionRate:        0.5,
			AvgOps:                2.5,
			EscalationCorrectness: 0.5,
		},
		Tasks: []SelftestTask{
			{ID: "1.1", Passed: true},
			{ID: "1.2", Passed: false}, // regressed
			{ID: "1.3", Passed: true},  // recovered
			{ID: "1.4", Passed: false}, // regressed
			{ID: "1.5", Passed: true},  // new task
		},
	}

	d := diffSelftest(&prior, &current, "/tmp/prior.json")

	if d.PriorReport != "/tmp/prior.json" {
		t.Errorf("prior report path: %q", d.PriorReport)
	}
	if !selftestAlmostEqual(d.CompletionRateDelta, -0.3) {
		t.Errorf("completion delta: %v", d.CompletionRateDelta)
	}
	if !selftestAlmostEqual(d.AvgOpsDelta, 0.5) {
		t.Errorf("avg ops delta: %v", d.AvgOpsDelta)
	}
	if !selftestAlmostEqual(d.EscalationDelta, -0.5) {
		t.Errorf("escalation delta: %v", d.EscalationDelta)
	}
	if strings.Join(d.Regressed, ",") != "1.2,1.4" {
		t.Errorf("regressed: %v", d.Regressed)
	}
	if strings.Join(d.Recovered, ",") != "1.3" {
		t.Errorf("recovered: %v", d.Recovered)
	}
	if strings.Join(d.NewTasks, ",") != "1.5" {
		t.Errorf("new: %v", d.NewTasks)
	}
	if len(d.DroppedTasks) != 0 {
		t.Errorf("dropped: %v", d.DroppedTasks)
	}
}

// TestSelftest_HandlesMissingPrior exercises the "no prior selftest_*.json
// in the output directory" branch end-to-end: findPriorSelftest must
// return (nil, "") and a full parse → metrics → write cycle must
// succeed with no diff section in the JSON.
func TestSelftest_HandlesMissingPrior(t *testing.T) {
	dir := t.TempDir()

	jsonl := writeJSONL(t, dir, "run.jsonl", []string{
		`{"ts":"2026-05-17T10:00:00Z","step":"1.1","outcome":"pass","note":"ok","extra_field":"tolerated"}`,
		`{"ts":"2026-05-17T10:00:01Z","step":"1.2","outcome":"escalate-paywall","note":"wsj"}`,
	})
	group := writeGroup(t, dir, map[string]string{
		"1.1": "no",
		"1.2": "yes",
	})

	prior, priorPath := findPriorSelftest(dir)
	if prior != nil || priorPath != "" {
		t.Fatalf("expected no prior, got %v %q", prior, priorPath)
	}

	steps, err := parseSelftestJSONL(jsonl)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	exp, err := loadExpectedEscalations(group)
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	tasks := collapseSteps(steps, exp)
	m := computeSelftestMetrics(tasks)
	if m.TotalTasks != 2 || m.PassedTasks != 2 {
		t.Fatalf("metrics: %+v", m)
	}
	if !m.EscalationApplicable || !selftestAlmostEqual(m.EscalationCorrectness, 1.0) {
		t.Errorf("escalation: %+v", m)
	}

	report := SelftestReport{
		Version:    1,
		CapturedAt: "2026-05-17T10:00:00Z",
		InputPath:  jsonl,
		GroupPath:  group,
		Metrics:    m,
		Tasks:      tasks,
	}
	out := filepath.Join(dir, "selftest_20260517-100000.json")
	if err := writeSelftestJSON(out, report); err != nil {
		t.Fatalf("write json: %v", err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var roundTrip SelftestReport
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if roundTrip.Diff != nil {
		t.Errorf("expected no diff in JSON when prior missing, got %+v", roundTrip.Diff)
	}
	md := renderSelftestMarkdown(report)
	if !strings.Contains(md, "Completion rate") {
		t.Errorf("markdown missing headline: %q", md)
	}
	if strings.Contains(md, "## Diff vs prior") {
		t.Errorf("markdown should not have diff section: %q", md)
	}
}

// selftestAlmostEqual is a tiny epsilon comparator for the float metrics —
// avoids cross-platform fp drift in test failures.
func selftestAlmostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

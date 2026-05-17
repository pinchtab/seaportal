package main

// selftest is the agent-scoreboard lane of seabench. It parses a JSONL
// transcript produced by `tests/optimization/record.sh` (one line per
// task step) and emits three headline numbers:
//
//   - completion_rate     = passed_tasks / total_tasks
//   - avg_ops             = mean(steps_per_task)
//   - escalation_correctness = correct_escalations / total_escalations
//
// where a task is "passed" if its final outcome is one of pass /
// escalate / escalate-paywall AND, for the escalation cases, the task's
// declared `expected_escalation` is `yes`. `expected_escalation` lives
// inline in `tests/optimization/group-selftest.md` next to each task.
//
// No Claude API integration here. The subagent run is orchestrated by
// the shell + seaportal-opt skill; this subcommand only consumes its
// JSONL output. Default mode invokes `tests/optimization/selftest.sh`,
// but `--input <path>` skips orchestration and replays an existing
// JSONL — used by CI and by the unit tests.

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SelftestStep is a single line of record.sh output. Fields that
// record.sh might add later are tolerated by json.Unmarshal because we
// ignore unknown keys (default behaviour).
type SelftestStep struct {
	TS      string `json:"ts"`
	Step    string `json:"step"`
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

// SelftestTask is one task's collapsed view: ordered steps + the
// outcome we treat as "final" for scoring.
type SelftestTask struct {
	ID                  string         `json:"id"`
	Steps               []SelftestStep `json:"steps"`
	OpCount             int            `json:"op_count"`
	FinalOutcome        string         `json:"final_outcome"`
	ExpectedEscalation  string         `json:"expected_escalation"` // yes | no | unknown
	Passed              bool           `json:"passed"`
	EscalationAttempted bool           `json:"escalation_attempted"`
	EscalationCorrect   bool           `json:"escalation_correct"`
}

// SelftestMetrics is the three-number headline computed from tasks.
type SelftestMetrics struct {
	TotalTasks            int     `json:"total_tasks"`
	PassedTasks           int     `json:"passed_tasks"`
	CompletionRate        float64 `json:"completion_rate"`
	AvgOps                float64 `json:"avg_ops"`
	TotalEscalations      int     `json:"total_escalations"`
	CorrectEscalations    int     `json:"correct_escalations"`
	EscalationCorrectness float64 `json:"escalation_correctness"`
	EscalationApplicable  bool    `json:"escalation_applicable"`
}

// SelftestDiff is the comparison against the most-recent prior run.
type SelftestDiff struct {
	PriorReport         string   `json:"prior_report,omitempty"`
	CompletionRateDelta float64  `json:"completion_rate_delta"`
	AvgOpsDelta         float64  `json:"avg_ops_delta"`
	EscalationDelta     float64  `json:"escalation_correctness_delta"`
	Regressed           []string `json:"regressed,omitempty"` // tasks that went pass→fail
	Recovered           []string `json:"recovered,omitempty"` // tasks that went fail→pass
	NewTasks            []string `json:"new_tasks,omitempty"`
	DroppedTasks        []string `json:"dropped_tasks,omitempty"`
}

// SelftestReport is the on-disk JSON shape for a selftest run.
type SelftestReport struct {
	Version    int             `json:"version"`
	CapturedAt string          `json:"captured_at"`
	GitSHA     string          `json:"git_sha"`
	InputPath  string          `json:"input_path"`
	GroupPath  string          `json:"group_path,omitempty"`
	Metrics    SelftestMetrics `json:"metrics"`
	Tasks      []SelftestTask  `json:"tasks"`
	Diff       *SelftestDiff   `json:"diff,omitempty"`
}

func runSelftest(args []string) {
	fs := flag.NewFlagSet("selftest", flag.ExitOnError)
	input := fs.String("input", "", "Replay a prior JSONL transcript instead of invoking selftest.sh")
	groupPath := fs.String("group", "tests/optimization/group-selftest.md", "Path to the curated group file with expected_escalation metadata")
	output := fs.String("output", "tests/bench/reports", "Output directory for the JSON + Markdown reports")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	jsonlPath := *input
	if jsonlPath == "" {
		var err error
		jsonlPath, err = invokeSelftestScript(*output)
		if err != nil {
			fmt.Fprintln(os.Stderr, "selftest: invoke script:", err)
			os.Exit(1)
		}
		if jsonlPath == "" {
			fmt.Fprintln(os.Stderr, "selftest: subagent did not produce a JSONL transcript.")
			fmt.Fprintln(os.Stderr, "         Re-run with --input <path> to replay a prior or synthetic transcript.")
			os.Exit(0)
		}
	}

	expected, gerr := loadExpectedEscalations(*groupPath)
	if gerr != nil {
		// Missing or unreadable group file is non-fatal: every task
		// just gets expected=unknown and escalation metric becomes N/A.
		fmt.Fprintf(os.Stderr, "selftest: %v (escalation correctness will be N/A)\n", gerr)
		expected = map[string]string{}
	}

	steps, perr := parseSelftestJSONL(jsonlPath)
	if perr != nil {
		fmt.Fprintln(os.Stderr, "selftest: parse:", perr)
		os.Exit(1)
	}

	tasks := collapseSteps(steps, expected)
	metrics := computeSelftestMetrics(tasks)

	report := SelftestReport{
		Version:    1,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		GitSHA:     gitSHA(),
		InputPath:  jsonlPath,
		GroupPath:  *groupPath,
		Metrics:    metrics,
		Tasks:      tasks,
	}

	if err := os.MkdirAll(*output, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "selftest: mkdir output:", err)
		os.Exit(1)
	}

	if prior, priorPath := findPriorSelftest(*output); prior != nil {
		report.Diff = diffSelftest(prior, &report, priorPath)
	}

	ts := time.Now().UTC().Format("20060102-150405")
	jsonOut := filepath.Join(*output, fmt.Sprintf("selftest_%s.json", ts))
	mdOut := filepath.Join(*output, fmt.Sprintf("selftest_%s.md", ts))

	if err := writeSelftestJSON(jsonOut, report); err != nil {
		fmt.Fprintln(os.Stderr, "selftest: write json:", err)
		os.Exit(1)
	}
	if err := atomicWrite(mdOut, renderSelftestMarkdown(report)); err != nil {
		fmt.Fprintln(os.Stderr, "selftest: write markdown:", err)
		os.Exit(1)
	}

	fmt.Println("wrote", jsonOut)
	fmt.Println("wrote", mdOut)
	fmt.Printf("selftest: completion=%.3f avg_ops=%.2f escalation=%s\n",
		metrics.CompletionRate, metrics.AvgOps, formatEscalationRate(metrics))
}

// invokeSelftestScript runs tests/optimization/selftest.sh with a
// pre-computed SEAPORTAL_REPORT_FILE pointing into the output dir so we
// know exactly where to parse from afterwards. Returns "" if the script
// exited 0 but produced no records (no subagent runtime available).
func invokeSelftestScript(outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102_150405")
	abs, err := filepath.Abs(outputDir)
	if err != nil {
		return "", err
	}
	jsonl := filepath.Join(abs, fmt.Sprintf("selftest_%s.jsonl", ts))

	script := "tests/optimization/selftest.sh"
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("missing %s: %w", script, err)
	}

	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "SEAPORTAL_REPORT_FILE="+jsonl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("selftest.sh failed: %w", err)
	}

	info, statErr := os.Stat(jsonl)
	if statErr != nil || info.Size() == 0 {
		return "", nil
	}
	return jsonl, nil
}

// parseSelftestJSONL reads one JSON object per non-empty line and
// returns the steps in input order. Tolerant of trailing blank lines
// and of unknown fields (json.Unmarshal default).
func parseSelftestJSONL(path string) ([]SelftestStep, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var steps []SelftestStep
	scanner := bufio.NewScanner(f)
	// record.sh writes a single JSON object per line; bump the buffer so
	// long `note` fields don't trip the default 64k limit.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var s SelftestStep
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		steps = append(steps, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return steps, nil
}

// collapseSteps groups steps by task. A "task" is the dotted prefix of
// the step id (e.g. "1.3" → task "1"). The final outcome of the task
// is the outcome of its last recorded step; the op count is the
// number of steps in the group (a proxy for seaportal invocations,
// since the agent records one step per logical action).
func collapseSteps(steps []SelftestStep, expected map[string]string) []SelftestTask {
	groups := make(map[string][]SelftestStep)
	order := make([]string, 0)
	for _, s := range steps {
		id := taskIDForStep(s.Step)
		if _, ok := groups[id]; !ok {
			order = append(order, id)
		}
		groups[id] = append(groups[id], s)
	}

	tasks := make([]SelftestTask, 0, len(order))
	for _, id := range order {
		gs := groups[id]
		final := ""
		if len(gs) > 0 {
			final = gs[len(gs)-1].Outcome
		}
		exp := expected[id]
		if exp == "" {
			exp = "unknown"
		}
		passed, attempted, correct := scoreTask(final, exp)
		tasks = append(tasks, SelftestTask{
			ID:                  id,
			Steps:               gs,
			OpCount:             len(gs),
			FinalOutcome:        final,
			ExpectedEscalation:  exp,
			Passed:              passed,
			EscalationAttempted: attempted,
			EscalationCorrect:   correct,
		})
	}
	return tasks
}

// taskIDForStep returns the task identifier for a step. record.sh uses
// dotted ids like "1.3" or "1.10"; the leading segment groups task vs
// step, but `group-selftest.md` headers are written as "1.3" rather
// than just "1", so we return the full prefix up to the LAST dot. This
// also gracefully handles ids that already are task ids (no dot).
func taskIDForStep(step string) string {
	if step == "" {
		return ""
	}
	// In `group-selftest.md` every header is "1.N". record.sh step ids
	// follow the same pattern. The whole id IS the task id — there are
	// no sub-steps below it in selftest. Strip leading/trailing space
	// and collapse to canonical form.
	return strings.TrimSpace(step)
}

// scoreTask collapses the final outcome + expectation into the three
// booleans the metrics layer needs.
//
//   - passed: task counted as a success
//   - attempted: agent decided to escalate (denominator for correctness)
//   - correct: agent escalated AND ground truth says it should have
//
// Outcomes:
//   - pass                : success when expected_escalation is "no" or "unknown"
//   - escalate            : success only when expected_escalation == "yes"
//   - escalate-paywall    : success only when expected_escalation == "yes"
//   - fail / anything else: not a success
func scoreTask(final, expected string) (passed, attempted, correct bool) {
	switch final {
	case "pass":
		// A `pass` when escalation was expected is still credit for the
		// agent — extracting content from a hard page is the goal. But
		// it doesn't count toward the escalation-correctness numerator.
		passed = true
	case "escalate", "escalate-paywall":
		attempted = true
		if expected == "yes" {
			passed = true
			correct = true
		}
	}
	return
}

// computeSelftestMetrics rolls task-level booleans into the three
// scoreboard numbers. Escalation correctness is N/A (zero) when no
// task escalated; callers should use EscalationApplicable to decide
// whether to render a number or "N/A".
func computeSelftestMetrics(tasks []SelftestTask) SelftestMetrics {
	m := SelftestMetrics{TotalTasks: len(tasks)}
	if len(tasks) == 0 {
		return m
	}
	totalOps := 0
	for _, t := range tasks {
		if t.Passed {
			m.PassedTasks++
		}
		totalOps += t.OpCount
		if t.EscalationAttempted {
			m.TotalEscalations++
			if t.EscalationCorrect {
				m.CorrectEscalations++
			}
		}
	}
	m.CompletionRate = float64(m.PassedTasks) / float64(m.TotalTasks)
	m.AvgOps = float64(totalOps) / float64(m.TotalTasks)
	if m.TotalEscalations > 0 {
		m.EscalationCorrectness = float64(m.CorrectEscalations) / float64(m.TotalEscalations)
		m.EscalationApplicable = true
	}
	return m
}

// expectedEscalationRe matches the inline metadata line in
// group-selftest.md: `**Expected escalation:** yes|no` (case-insensitive
// value). The most-recent preceding header `### 1.3 …` provides the
// task id we attach the value to.
var (
	expectedEscalationRe = regexp.MustCompile(`(?i)^\s*\*\*Expected escalation:\*\*\s*(yes|no)\s*$`)
	taskHeaderRe         = regexp.MustCompile(`^###\s+(\S+)\s+`)
)

// loadExpectedEscalations parses `tests/optimization/group-selftest.md`
// and builds task_id → "yes"|"no". Unknown / missing values are simply
// absent from the map; the metrics layer treats absent as "unknown".
func loadExpectedEscalations(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out, fmt.Errorf("open group file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1*1024*1024)
	currentID := ""
	for scanner.Scan() {
		line := scanner.Text()
		if m := taskHeaderRe.FindStringSubmatch(line); m != nil {
			currentID = m[1]
			continue
		}
		if m := expectedEscalationRe.FindStringSubmatch(line); m != nil {
			if currentID != "" {
				out[currentID] = strings.ToLower(m[1])
			}
		}
	}
	return out, scanner.Err()
}

// findPriorSelftest returns the most-recent prior selftest_*.json in
// the output dir, parsed. Returns (nil, "") if none exists.
func findPriorSelftest(outputDir string) (*SelftestReport, string) {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return nil, ""
	}
	candidates := make([]string, 0)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "selftest_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		candidates = append(candidates, name)
	}
	if len(candidates) == 0 {
		return nil, ""
	}
	sort.Strings(candidates)
	prior := candidates[len(candidates)-1]
	path := filepath.Join(outputDir, prior)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}
	var r SelftestReport
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, ""
	}
	return &r, path
}

// diffSelftest builds the diff envelope by comparing task-level Passed
// flags between prior and current. Returns deltas + lists of
// regressed/recovered/new/dropped task ids. Lists are sorted for
// stable rendering.
func diffSelftest(prior, current *SelftestReport, priorPath string) *SelftestDiff {
	d := &SelftestDiff{
		PriorReport:         priorPath,
		CompletionRateDelta: current.Metrics.CompletionRate - prior.Metrics.CompletionRate,
		AvgOpsDelta:         current.Metrics.AvgOps - prior.Metrics.AvgOps,
		EscalationDelta:     current.Metrics.EscalationCorrectness - prior.Metrics.EscalationCorrectness,
	}
	priorByID := make(map[string]SelftestTask, len(prior.Tasks))
	for _, t := range prior.Tasks {
		priorByID[t.ID] = t
	}
	currentByID := make(map[string]SelftestTask, len(current.Tasks))
	for _, t := range current.Tasks {
		currentByID[t.ID] = t
	}
	for id, cur := range currentByID {
		old, ok := priorByID[id]
		if !ok {
			d.NewTasks = append(d.NewTasks, id)
			continue
		}
		switch {
		case old.Passed && !cur.Passed:
			d.Regressed = append(d.Regressed, id)
		case !old.Passed && cur.Passed:
			d.Recovered = append(d.Recovered, id)
		}
	}
	for id := range priorByID {
		if _, ok := currentByID[id]; !ok {
			d.DroppedTasks = append(d.DroppedTasks, id)
		}
	}
	sort.Strings(d.Regressed)
	sort.Strings(d.Recovered)
	sort.Strings(d.NewTasks)
	sort.Strings(d.DroppedTasks)
	return d
}

func writeSelftestJSON(path string, r SelftestReport) error {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, string(raw)+"\n")
}

func formatEscalationRate(m SelftestMetrics) string {
	if !m.EscalationApplicable {
		return "N/A"
	}
	return fmt.Sprintf("%.3f (%d/%d)", m.EscalationCorrectness, m.CorrectEscalations, m.TotalEscalations)
}

func renderSelftestMarkdown(r SelftestReport) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# SeaPortal Agent Selftest Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Captured: %s\n", r.CapturedAt)
	fmt.Fprintf(&b, "- Git SHA: `%s`\n", r.GitSHA)
	fmt.Fprintf(&b, "- Input: `%s`\n", r.InputPath)
	if r.GroupPath != "" {
		fmt.Fprintf(&b, "- Group: `%s`\n", r.GroupPath)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Headline")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Metric | Value |")
	fmt.Fprintln(&b, "|---|---|")
	fmt.Fprintf(&b, "| Completion rate | %.3f (%d/%d) |\n",
		r.Metrics.CompletionRate, r.Metrics.PassedTasks, r.Metrics.TotalTasks)
	fmt.Fprintf(&b, "| Avg ops per task | %.2f |\n", r.Metrics.AvgOps)
	fmt.Fprintf(&b, "| Escalation correctness | %s |\n", formatEscalationRate(r.Metrics))
	fmt.Fprintln(&b)

	if r.Diff != nil {
		fmt.Fprintln(&b, "## Diff vs prior")
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "- Prior report: `%s`\n", r.Diff.PriorReport)
		fmt.Fprintf(&b, "- Completion delta: %+.3f\n", r.Diff.CompletionRateDelta)
		fmt.Fprintf(&b, "- Avg-ops delta: %+.2f\n", r.Diff.AvgOpsDelta)
		fmt.Fprintf(&b, "- Escalation delta: %+.3f\n", r.Diff.EscalationDelta)
		if len(r.Diff.Regressed) > 0 {
			fmt.Fprintf(&b, "- Regressed: %s\n", strings.Join(r.Diff.Regressed, ", "))
		}
		if len(r.Diff.Recovered) > 0 {
			fmt.Fprintf(&b, "- Recovered: %s\n", strings.Join(r.Diff.Recovered, ", "))
		}
		if len(r.Diff.NewTasks) > 0 {
			fmt.Fprintf(&b, "- New: %s\n", strings.Join(r.Diff.NewTasks, ", "))
		}
		if len(r.Diff.DroppedTasks) > 0 {
			fmt.Fprintf(&b, "- Dropped: %s\n", strings.Join(r.Diff.DroppedTasks, ", "))
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## Per-task")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Task | Ops | Final | Expected escalation | Passed | Esc. correct |")
	fmt.Fprintln(&b, "|---|---|---|---|---|---|")
	for _, t := range r.Tasks {
		passed := "✗"
		if t.Passed {
			passed = "✓"
		}
		escCorrect := "—"
		if t.EscalationAttempted {
			escCorrect = "✗"
			if t.EscalationCorrect {
				escCorrect = "✓"
			}
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s |\n",
			t.ID, t.OpCount, t.FinalOutcome, t.ExpectedEscalation, passed, escCorrect)
	}
	return b.String()
}

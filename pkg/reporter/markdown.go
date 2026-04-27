package reporter

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// MarkdownReporter writes experiment reports as Markdown,
// suitable for GitHub/GitLab PR comments and Jira tickets.
type MarkdownReporter struct{}

// WriteReport renders a collection of experiment reports as Markdown.
func (r *MarkdownReporter) WriteReport(w io.Writer, reports []ExperimentReport) error {
	summary := ComputeSummary(reports)
	var b strings.Builder

	// Title and header stats.
	b.WriteString("# Chaos Experiment Report\n\n")
	fmt.Fprintf(&b,
		"**Generated:** %s | **Experiments:** %d | **Pass Rate:** %.1f%%\n\n",
		time.Now().UTC().Format(time.RFC3339),
		summary.Total,
		summary.PassRate*100,
	)

	// Summary verdict table (non-zero only).
	b.WriteString("## Summary\n\n")
	b.WriteString("| Verdict | Count |\n")
	b.WriteString("|---------|-------|\n")
	verdicts := []struct {
		name  string
		count int
	}{
		{"Resilient", summary.Resilient},
		{"Degraded", summary.Degraded},
		{"Failed", summary.Failed},
		{"Inconclusive", summary.Inconclusive},
	}
	for _, v := range verdicts {
		if v.count > 0 {
			fmt.Fprintf(&b, "| %s | %d |\n", v.name, v.count)
		}
	}
	b.WriteString("\n")

	// Results per experiment.
	b.WriteString("## Results\n\n")
	for _, report := range reports {
		writeExperiment(&b, report)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func writeExperiment(b *strings.Builder, report ExperimentReport) {
	fmt.Fprintf(b, "### %s\n\n", report.Experiment)

	// Field table.
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	fmt.Fprintf(b, "| Component | %s |\n", report.Target.Component)
	if report.Tier > 0 {
		fmt.Fprintf(b, "| Tier | %d |\n", report.Tier)
	}
	fmt.Fprintf(b, "| Injection | %s |\n", report.Injection.Type)
	fmt.Fprintf(b, "| Verdict | %s |\n", string(report.Evaluation.Verdict))
	fmt.Fprintf(b, "| Recovery | %s |\n",
		formatRecovery(report.Evaluation.RecoveryTime, report.Evaluation.ReconcileCycles))
	b.WriteString("\n")

	// Collapsible details.
	b.WriteString("<details>\n<summary>Details</summary>\n\n")
	writeDetails(b, report)
	b.WriteString("</details>\n\n")
}

func writeDetails(b *strings.Builder, report ExperimentReport) {
	// Injection parameters.
	if params := formatMapSorted(report.Injection.Details); params != "" {
		fmt.Fprintf(b, "**Injection Parameters:** %s\n\n", params)
	}

	// Steady state.
	fmt.Fprintf(b, "**Steady State:** Pre: %s | Post: %s\n\n",
		formatCheckResult(report.SteadyState.Pre),
		formatCheckResult(report.SteadyState.Post),
	)

	// Deviations.
	if len(report.Evaluation.Deviations) == 0 {
		b.WriteString("**Deviations:** None\n\n")
	} else {
		b.WriteString("**Deviations:**\n\n")
		for _, d := range report.Evaluation.Deviations {
			fmt.Fprintf(b, "- `%s`: %s\n", d.Type, d.Detail)
		}
		b.WriteString("\n")
	}

	// Cleanup error.
	if report.CleanupError != "" {
		fmt.Fprintf(b, "**Cleanup Error:** %s\n\n", report.CleanupError)
	}

	// Collateral findings.
	if len(report.Collateral) > 0 {
		b.WriteString("**Collateral Findings:**\n\n")
		for _, c := range report.Collateral {
			fmt.Fprintf(b, "- %s/%s: %s\n", c.Operator, c.Component, verdictFromBool(c.Passed))
		}
		b.WriteString("\n")
	}
}

// formatRecovery formats recovery time and cycle count for display.
// Returns "N/A" when both are zero, pluralizes "cycle"/"cycles".
func formatRecovery(d time.Duration, cycles int) string {
	if d == 0 && cycles == 0 {
		return "N/A"
	}
	s := fmt.Sprintf("%.1fs", d.Seconds())
	if cycles > 0 {
		unit := "cycle"
		if cycles != 1 {
			unit = "cycles"
		}
		s += fmt.Sprintf(" (%d %s)", cycles, unit)
	}
	return s
}

// formatCheckResult converts a steady-state check value to a display string.
// Returns "PASS" for true, "FAIL" for false, "N/A" for anything else.
func formatCheckResult(v any) string {
	if b, ok := v.(bool); ok {
		if b {
			return "PASS"
		}
		return "FAIL"
	}
	return "N/A"
}

// formatMapSorted formats a map as "k=v, k=v" with keys sorted alphabetically.
func formatMapSorted(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(pairs, ", ")
}

func verdictFromBool(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

// Compile-time check that MarkdownReporter implements Reporter.
var _ Reporter = (*MarkdownReporter)(nil)

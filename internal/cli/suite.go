package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	v1alpha1 "github.com/opendatahub-io/operator-chaos/api/v1alpha1"
	"github.com/opendatahub-io/operator-chaos/pkg/evaluator"
	"github.com/opendatahub-io/operator-chaos/pkg/experiment"
	"github.com/opendatahub-io/operator-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/operator-chaos/pkg/reporter"
	"github.com/spf13/cobra"
)

// suiteResult captures the outcome of running a single experiment in the suite.
type suiteResult struct {
	file       string
	name       string
	verdict    string
	status     string // "pass", "fail", "skip"
	err        error
	orchResult *orchestrator.ExperimentResult
	target     v1alpha1.TargetSpec
	tier       int32
}

func newSuiteCommand() *cobra.Command {
	var (
		knowledgePaths  []string
		knowledgeDir    string
		profile         string
		reportDir       string
		dryRun          bool
		timeout         time.Duration
		parallel        int
		distributedLock bool
		lockNamespace   string
		maxTier         int32
		cooldown        time.Duration
	)

	cmd := &cobra.Command{
		Use:   "suite <experiments-directory>",
		Short: "Run all experiments in a directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if parallel < 1 {
				return fmt.Errorf("--parallel must be >= 1, got %d", parallel)
			}
			if maxTier < 0 || maxTier > v1alpha1.MaxTier {
				return fmt.Errorf("--max-tier must be 0 (no filter) or between %d and %d", v1alpha1.MinTier, v1alpha1.MaxTier)
			}

			if profile != "" {
				pp, err := resolveProfile(profile)
				if err != nil {
					return err
				}
				if pp.KnowledgeDir != "" && knowledgeDir == "" && len(knowledgePaths) == 0 {
					knowledgeDir = pp.KnowledgeDir
				}
			}

			dir := args[0]

			// Find all YAML files
			entries, err := os.ReadDir(dir)
			if err != nil {
				return fmt.Errorf("reading directory %s: %w", dir, err)
			}

			var experimentFiles []string
			for _, entry := range entries {
				if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
					experimentFiles = append(experimentFiles, filepath.Join(dir, entry.Name()))
				}
			}

			if len(experimentFiles) == 0 {
				return fmt.Errorf("no experiment files found in %s", dir)
			}

			fmt.Fprintf(os.Stderr, "Found %d experiments in %s\n\n", len(experimentFiles), dir)

			// Build orchestrator once for all experiments (when not dry-run)
			verbose, _ := cmd.Flags().GetBool("verbose")
			var deps *orchestratorDeps
			if !dryRun {
				deps, err = buildOrchestrator(knowledgePaths, knowledgeDir, dryRun, reportDir, distributedLock, lockNamespace, verbose)
				if err != nil {
					return fmt.Errorf("building orchestrator: %w", err)
				}
			}

			// Only override namespace when the user explicitly passes --namespace.
			// The persistent flag has a default value, so always reading it
			// would silently override per-check namespaces in YAML.
			var namespace string
			if cmd.Flags().Changed("namespace") {
				namespace, _ = cmd.Flags().GetString("namespace")
			}

			var results []suiteResult

			if parallel > 1 && !dryRun {
				if cooldown > 0 {
					fmt.Fprintf(os.Stderr, "Warning: --cooldown is ignored when --parallel > 1\n")
				}
				results = runParallel(cmd.Context(), experimentFiles, deps, timeout, parallel, namespace, maxTier)
			} else {
				results = runSequential(cmd.Context(), experimentFiles, deps, dryRun, timeout, namespace, maxTier, cooldown)
			}

			// Print results and count verdicts
			passed := 0
			failed := 0
			skipped := 0

			for _, r := range results {
				switch r.status {
				case "pass":
					passed++
				case "fail":
					failed++
				case "skip":
					skipped++
				}
				fmt.Println(r.verdict)
			}

			// Tier distribution
			tierCounts := make(map[int32]int)
			for _, r := range results {
				if r.tier > 0 {
					tierCounts[r.tier]++
				}
			}
			tierSuffix := ""
			if len(tierCounts) > 0 {
				tierSuffix = " | tiers:"
				for t := int32(v1alpha1.MinTier); t <= int32(v1alpha1.MaxTier); t++ {
					if c, ok := tierCounts[t]; ok {
						tierSuffix += fmt.Sprintf(" T%d=%d", t, c)
					}
				}
			}

			fmt.Printf("\nSuite summary: %d passed, %d failed, %d skipped (total: %d)%s\n",
				passed, failed, skipped, len(experimentFiles), tierSuffix)

			// Generate JUnit report if reportDir is specified
			if reportDir != "" {
				if err := writeSuiteJUnitReport(reportDir, dir, results); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write JUnit report: %v\n", err)
				}
			}

			// Generate HTML report if reportDir is specified
			if reportDir != "" {
				if err := writeSuiteHTMLReport(reportDir, results); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to write HTML report: %v\n", err)
				}
			}

			if failed > 0 {
				return fmt.Errorf("%d experiment(s) failed", failed)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&profile, "profile", "", "named profile (resolves knowledge directory automatically)")
	cmd.Flags().StringArrayVar(&knowledgePaths, "knowledge", nil, "path to operator knowledge YAML (repeatable)")
	cmd.Flags().StringVar(&knowledgeDir, "knowledge-dir", "", "directory of operator knowledge YAMLs")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "directory for report output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without running")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "timeout per experiment")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "max concurrent experiments")
	cmd.Flags().BoolVar(&distributedLock, "distributed-lock", false, "use Kubernetes Lease-based distributed locking")
	cmd.Flags().StringVar(&lockNamespace, "lock-namespace", v1alpha1.DefaultNamespace, "namespace for distributed lock leases")
	cmd.Flags().Int32Var(&maxTier, "max-tier", 0, "skip experiments above this tier (0 = no filter)")
	cmd.Flags().DurationVar(&cooldown, "cooldown", 0, "delay between sequential experiments to allow cluster recovery (e.g. 30s)")

	return cmd
}

// runSequential executes experiments one at a time with an optional cooldown
// between experiments to allow the cluster to stabilize after disruptive injections.
func runSequential(parentCtx context.Context, files []string, deps *orchestratorDeps, dryRun bool, timeout time.Duration, namespace string, maxTier int32, cooldown time.Duration) []suiteResult {
	results := make([]suiteResult, 0, len(files))

	for i, file := range files {
		r := runSingleExperiment(parentCtx, file, deps, dryRun, timeout, namespace, maxTier)
		results = append(results, r)

		if cooldown > 0 && i < len(files)-1 && !dryRun && r.status != "skip" {
			fmt.Fprintf(os.Stderr, "Cooldown: waiting %s before next experiment...\n", cooldown)
			select {
			case <-time.After(cooldown):
			case <-parentCtx.Done():
				return results
			}
		}
	}

	return results
}

// runParallel executes experiments concurrently with a semaphore limiting concurrency.
func runParallel(parentCtx context.Context, files []string, deps *orchestratorDeps, timeout time.Duration, maxConcurrent int, namespace string, maxTier int32) []suiteResult {
	results := make([]suiteResult, len(files))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, file := range files {
		wg.Add(1)
		go func(idx int, f string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			results[idx] = runSingleExperiment(parentCtx, f, deps, false, timeout, namespace, maxTier)
		}(i, file)
	}

	wg.Wait()
	return results
}

// runSingleExperiment loads, validates, and optionally executes a single experiment file.
func runSingleExperiment(parentCtx context.Context, file string, deps *orchestratorDeps, dryRun bool, timeout time.Duration, namespace string, maxTier int32) suiteResult {
	r := suiteResult{file: file}

	exp, err := experiment.Load(file)
	if err != nil {
		r.name = filepath.Base(file)
		r.verdict = fmt.Sprintf("SKIP  %s: %v", filepath.Base(file), err)
		r.status = "skip"
		r.err = err
		return r
	}
	r.name = exp.Name
	r.target = exp.Spec.Target
	r.tier = exp.Spec.Tier

	// Skip experiments above the max tier
	if maxTier > 0 && exp.Spec.Tier > maxTier {
		r.verdict = fmt.Sprintf("SKIP  %s (tier %d > max-tier %d)", exp.Name, exp.Spec.Tier, maxTier)
		r.status = "skip"
		return r
	}

	// Override namespace from CLI flag
	if namespace != "" {
		overrideExperimentNamespace(exp, namespace)
	}

	errs := experiment.Validate(exp)
	if len(errs) > 0 {
		r.verdict = fmt.Sprintf("SKIP  %s: %d validation errors", filepath.Base(file), len(errs))
		r.status = "skip"
		return r
	}

	// In dry-run mode, just validate
	if dryRun {
		r.verdict = fmt.Sprintf("VALID %s (%s)", exp.Name, exp.Spec.Injection.Type)
		r.status = "pass"
		return r
	}

	// Execute experiment
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	result, runErr := deps.Orchestrator.Run(ctx, exp)
	cancel()

	if runErr != nil {
		r.verdict = fmt.Sprintf("FAIL  %s: %v", exp.Name, runErr)
		r.status = "fail"
		r.err = runErr
		return r
	}

	r.orchResult = result

	if result.CleanupError != "" {
		fmt.Fprintf(os.Stderr, "WARNING: cleanup error in %s: %s\n", exp.Name, result.CleanupError)
	}

	// Build enriched verdict string with recovery time and deviation count
	verdictStr := colorVerdict(string(result.Verdict))
	recoveryStr := "0s"
	deviationCount := 0
	if result.Evaluation != nil {
		recoveryStr = result.Evaluation.RecoveryTime.Round(time.Second).String()
		deviationCount = len(result.Evaluation.Deviations)
	}

	switch result.Verdict {
	case v1alpha1.Resilient:
		r.verdict = fmt.Sprintf("PASS  %s (%s, %s recovery, %d deviations)",
			exp.Name, verdictStr, recoveryStr, deviationCount)
		r.status = "pass"
	case v1alpha1.Degraded, v1alpha1.Failed:
		r.verdict = fmt.Sprintf("FAIL  %s (%s, %s recovery, %d deviations)",
			exp.Name, verdictStr, recoveryStr, deviationCount)
		r.status = "fail"
	case v1alpha1.Inconclusive:
		r.verdict = fmt.Sprintf("SKIP  %s (%s, %s recovery, %d deviations)",
			exp.Name, verdictStr, recoveryStr, deviationCount)
		r.status = "skip"
	default:
		r.verdict = fmt.Sprintf("FAIL  %s (%s, %s recovery, %d deviations)",
			exp.Name, verdictStr, recoveryStr, deviationCount)
		r.status = "fail"
	}

	return r
}

// writeSuiteJUnitReport generates a JUnit XML report from suite results.
// It creates the report directory if needed and writes a combined report
// covering all experiments in the suite.
func writeSuiteJUnitReport(reportDir, suiteName string, results []suiteResult) error {
	if err := os.MkdirAll(reportDir, 0750); err != nil {
		return fmt.Errorf("creating report directory: %w", err)
	}

	outPath := filepath.Join(reportDir, "suite-results.xml")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating JUnit report file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Build ExperimentReport entries from suite results
	var reports []reporter.ExperimentReport
	for _, r := range results {
		report := suiteResultToReport(r)
		reports = append(reports, report)
	}

	junitReporter := reporter.NewJUnitReporter(f)
	if err := junitReporter.WriteSuite(filepath.Base(suiteName), reports); err != nil {
		return fmt.Errorf("writing JUnit XML: %w", err)
	}

	fmt.Fprintf(os.Stderr, "JUnit report written to %s\n", outPath)
	return nil
}

// writeSuiteHTMLReport generates an HTML report from suite results.
func writeSuiteHTMLReport(reportDir string, results []suiteResult) error {
	var reports []reporter.ExperimentReport
	for _, r := range results {
		reports = append(reports, suiteResultToReport(r))
	}

	outPath := filepath.Join(reportDir, "report.html")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating HTML report: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := reporter.NewHTMLReporter(Version)
	if err := h.WriteReport(f, reports); err != nil {
		return fmt.Errorf("writing HTML report: %w", err)
	}

	fmt.Fprintf(os.Stderr, "HTML report written to %s\n", outPath)
	return nil
}

// suiteResultToReport converts a suiteResult into a reporter.ExperimentReport.
// When the orchestrator produced a full report, it is used directly. Otherwise,
// a minimal report is synthesised from the suite result metadata.
func suiteResultToReport(r suiteResult) reporter.ExperimentReport {
	// If the orchestrator already produced a report, use it directly.
	if r.orchResult != nil && r.orchResult.Report != nil {
		return *r.orchResult.Report
	}

	// Build a minimal report for experiments that were skipped or failed
	// before the orchestrator could produce a full report.
	report := reporter.ExperimentReport{
		Experiment: r.name,
		Timestamp:  time.Now(),
		Tier:       r.tier,
		Target: reporter.TargetReport{
			Operator:  r.target.Operator,
			Component: r.target.Component,
		},
	}

	switch r.status {
	case "pass":
		report.Evaluation = evaluator.EvaluationResult{
			Verdict:    v1alpha1.Resilient,
			Confidence: "validated (dry-run or pre-execution)",
		}
	case "fail":
		errMsg := "experiment failed"
		if r.err != nil {
			errMsg = r.err.Error()
		}
		report.Evaluation = evaluator.EvaluationResult{
			Verdict:    v1alpha1.Failed,
			Confidence: errMsg,
		}
	case "skip":
		errMsg := "skipped"
		if r.err != nil {
			errMsg = r.err.Error()
		}
		report.Evaluation = evaluator.EvaluationResult{
			Verdict:    v1alpha1.Inconclusive,
			Confidence: errMsg,
		}
	}

	if r.orchResult != nil && r.orchResult.CleanupError != "" {
		report.CleanupError = r.orchResult.CleanupError
	}

	return report
}

package orchestrator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/reporter"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
)

// Orchestrator wires together all engines and manages the experiment
// lifecycle state machine: validation -> lock -> pre-check -> inject ->
// observe -> post-check -> evaluate -> report -> cleanup.
type Orchestrator struct {
	registry   *injection.Registry
	observer   observer.Observer
	reconciler *observer.ReconciliationChecker
	evaluator  *evaluator.Evaluator
	lock       safety.ExperimentLock
	knowledge  *model.OperatorKnowledge
	reportDir  string
	verbose    bool
	output     io.Writer
	logger     *slog.Logger
}

// OrchestratorConfig holds configuration for creating an Orchestrator.
type OrchestratorConfig struct {
	Registry   *injection.Registry
	Observer   observer.Observer
	Reconciler *observer.ReconciliationChecker
	Evaluator  *evaluator.Evaluator
	Lock       safety.ExperimentLock
	Knowledge  *model.OperatorKnowledge
	ReportDir  string
	Verbose    bool
	Logger     *slog.Logger
}

// ExperimentResult captures the outcome of running a chaos experiment.
type ExperimentResult struct {
	Experiment string                      `json:"experiment"`
	Phase      v1alpha1.ExperimentPhase    `json:"phase"`
	Verdict    v1alpha1.Verdict            `json:"verdict,omitempty"`
	Evaluation *evaluator.EvaluationResult `json:"evaluation,omitempty"`
	Report     *reporter.ExperimentReport  `json:"report,omitempty"`
	Error        string                      `json:"error,omitempty"`
	CleanupError string                      `json:"cleanupError,omitempty"`
}

// New creates a new Orchestrator with the given configuration.
func New(config OrchestratorConfig) *Orchestrator {
	output := io.Writer(os.Stdout)

	logger := config.Logger
	if logger == nil {
		if config.Verbose {
			logger = slog.New(slog.NewTextHandler(output, nil))
		} else {
			logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		}
	}

	return &Orchestrator{
		registry:   config.Registry,
		observer:   config.Observer,
		reconciler: config.Reconciler,
		evaluator:  config.Evaluator,
		lock:       config.Lock,
		knowledge:  config.Knowledge,
		reportDir:  config.ReportDir,
		verbose:    config.Verbose,
		output:     output,
		logger:     logger,
	}
}

// Run executes the full experiment lifecycle for the given ChaosExperiment.
// It proceeds through the phases: Pending -> SteadyStatePre -> Injecting ->
// Observing -> SteadyStatePost -> Evaluating -> Complete (or Aborted on error).
func (o *Orchestrator) Run(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*ExperimentResult, error) {
	result := &ExperimentResult{
		Experiment: exp.Metadata.Name,
		Phase:      v1alpha1.PhasePending,
	}

	// 1. Validate
	o.logger.Info("phase transition", "phase", "PENDING", "experiment", exp.Metadata.Name, "action", "validating")

	// Determine namespace early (needed for blast radius validation)
	namespace := exp.Metadata.Namespace
	if namespace == "" {
		namespace = "opendatahub"
	}

	// Determine target resource for forbidden-resource validation
	targetResource := exp.Spec.Target.Resource
	if targetResource == "" {
		targetResource = fmt.Sprintf("%s/%s", exp.Spec.Target.Component, exp.Metadata.Name)
	}

	// Check blast radius
	if err := safety.ValidateBlastRadius(exp.Spec.BlastRadius, namespace, targetResource, exp.Spec.Injection.Count); err != nil {
		result.Error = fmt.Sprintf("blast radius validation failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("blast radius: %w", err)
	}

	// Check danger level
	if err := safety.CheckDangerLevel(exp.Spec.Injection.DangerLevel, exp.Spec.BlastRadius.AllowDangerous); err != nil {
		result.Error = fmt.Sprintf("danger level check failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("danger level: %w", err)
	}

	// Get injector
	injector, err := o.registry.Get(exp.Spec.Injection.Type)
	if err != nil {
		result.Error = fmt.Sprintf("unknown injection type: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, err
	}

	// Validate injection spec
	if err := injector.Validate(exp.Spec.Injection, exp.Spec.BlastRadius); err != nil {
		result.Error = fmt.Sprintf("injection validation failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("injection validation: %w", err)
	}

	// Acquire experiment lock
	if err := o.lock.Acquire(ctx, exp.Spec.Target.Operator, exp.Metadata.Name); err != nil {
		result.Error = fmt.Sprintf("lock acquisition failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("lock: %w", err)
	}
	defer o.lock.Release(exp.Spec.Target.Operator)

	// Dry run check
	if exp.Spec.BlastRadius.DryRun {
		o.logger.Info("dry run", "injection", exp.Spec.Injection.Type, "operator", exp.Spec.Target.Operator, "component", exp.Spec.Target.Component)
		result.Phase = v1alpha1.PhaseComplete
		result.Verdict = v1alpha1.Inconclusive
		return result, nil
	}

	// 2. Steady State Pre-Check
	o.logger.Info("phase transition", "phase", "STEADY_STATE_PRE", "action", "checking baseline")
	result.Phase = v1alpha1.PhaseSteadyStatePre

	var preCheck *v1alpha1.CheckResult
	if len(exp.Spec.SteadyState.Checks) > 0 {
		var preCheckErr error
		preCheck, preCheckErr = o.observer.CheckSteadyState(ctx, exp.Spec.SteadyState.Checks, namespace)
		if preCheckErr != nil {
			result.Error = fmt.Sprintf("steady state pre-check error: %v", preCheckErr)
			result.Phase = v1alpha1.PhaseAborted
			return result, fmt.Errorf("pre-check: %w", preCheckErr)
		}
	} else {
		preCheck = &v1alpha1.CheckResult{Passed: true, Timestamp: time.Now()}
	}

	if !preCheck.Passed {
		o.logger.Warn("pre-check failed", "reason", "system not in steady state")
		evalResult := o.evaluator.Evaluate(preCheck, preCheck, false, 0, 0, exp.Spec.Hypothesis)
		result.Evaluation = evalResult
		result.Verdict = evalResult.Verdict
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("pre-check failed: system not in steady state")
	}

	// 3. Inject
	o.logger.Info("phase transition", "phase", "INJECTING", "injection", exp.Spec.Injection.Type)
	result.Phase = v1alpha1.PhaseInjecting

	cleanup, events, err := injector.Inject(ctx, exp.Spec.Injection, namespace)
	if err != nil {
		result.Error = fmt.Sprintf("injection failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("injection: %w", err)
	}

	// Ensure cleanup runs even if the parent context is cancelled
	defer func() {
		if cleanup != nil {
			o.logger.Info("phase transition", "phase", "CLEANUP")
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			if cleanErr := cleanup(cleanupCtx); cleanErr != nil {
				o.logger.Warn("cleanup warning", "error", cleanErr)
				result.CleanupError = cleanErr.Error()
			}
		}
	}()

	o.logger.Info("injection complete", "events", len(events))

	// 4. Observe
	o.logger.Info("phase transition", "phase", "OBSERVING", "action", "waiting for recovery")
	result.Phase = v1alpha1.PhaseObserving

	// Wait for observation period or recovery
	recoveryTimeout := exp.Spec.Hypothesis.RecoveryTimeout.Duration
	if recoveryTimeout == 0 {
		recoveryTimeout = 60 * time.Second
	}

	var reconciliationResult *observer.ReconciliationResult

	// Check reconciliation if knowledge model has the component
	if o.knowledge != nil {
		component := o.knowledge.GetComponent(exp.Spec.Target.Component)
		if component != nil && o.reconciler != nil {
			reconciliationResult, err = o.reconciler.CheckReconciliation(ctx, component, namespace, recoveryTimeout)
			if err != nil {
				o.logger.Warn("reconciliation check error", "error", err)
			}
		}
	}

	// 5. Steady State Post-Check
	o.logger.Info("phase transition", "phase", "STEADY_STATE_POST", "action", "verifying recovery")
	result.Phase = v1alpha1.PhaseSteadyStatePost

	var postCheck *v1alpha1.CheckResult
	if len(exp.Spec.SteadyState.Checks) > 0 {
		var postCheckErr error
		postCheck, postCheckErr = o.observer.CheckSteadyState(ctx, exp.Spec.SteadyState.Checks, namespace)
		if postCheckErr != nil {
			o.logger.Warn("post-check error", "error", postCheckErr)
			postCheck = &v1alpha1.CheckResult{Passed: false, Timestamp: time.Now()}
		}
	} else {
		postCheck = &v1alpha1.CheckResult{Passed: true, Timestamp: time.Now()}
	}

	// 6. Evaluate
	o.logger.Info("phase transition", "phase", "EVALUATING")
	result.Phase = v1alpha1.PhaseEvaluating

	allReconciled := true
	reconcileCycles := 0
	recoveryTime := time.Duration(0)

	if reconciliationResult != nil {
		allReconciled = reconciliationResult.AllReconciled
		reconcileCycles = reconciliationResult.ReconcileCycles
		recoveryTime = reconciliationResult.RecoveryTime
	}

	evalResult := o.evaluator.Evaluate(preCheck, postCheck, allReconciled, reconcileCycles, recoveryTime, exp.Spec.Hypothesis)
	result.Evaluation = evalResult
	result.Verdict = evalResult.Verdict

	// 7. Report
	report := reporter.ExperimentReport{
		Experiment: exp.Metadata.Name,
		Timestamp:  time.Now(),
		Target: reporter.TargetReport{
			Operator:  exp.Spec.Target.Operator,
			Component: exp.Spec.Target.Component,
			Resource:  exp.Spec.Target.Resource,
		},
		Injection: reporter.InjectionReport{
			Type:      string(exp.Spec.Injection.Type),
			Timestamp: time.Now(),
		},
		Evaluation: *evalResult,
	}
	result.Report = &report

	// Write JSON report if reportDir specified
	if o.reportDir != "" {
		reportPath := fmt.Sprintf("%s/%s-%s.json", o.reportDir, exp.Metadata.Name, time.Now().Format("20060102-150405"))
		r, err := reporter.NewJSONFileReporter(reportPath)
		if err == nil {
			_ = r.Write(report)
			_ = r.Close()
		}
	}

	o.logger.Info("phase transition", "phase", "COMPLETE", "verdict", evalResult.Verdict)
	result.Phase = v1alpha1.PhaseComplete

	return result, nil
}


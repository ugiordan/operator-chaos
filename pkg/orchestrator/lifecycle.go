package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/reporter"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	defaultCleanupTimeout = 30 * time.Second
)

// Orchestrator wires together all engines and manages the experiment
// lifecycle state machine: validation -> lock -> pre-check -> inject ->
// observe -> post-check -> evaluate -> report -> cleanup.
type Orchestrator struct {
	registry   *injection.Registry
	observer   observer.Observer
	reconciler observer.ReconciliationCheckerInterface
	evaluator  *evaluator.Evaluator
	lock       safety.ExperimentLock
	knowledge  *model.OperatorKnowledge
	k8sClient  client.Client
	reportDir string
	verbose   bool
	logger    *slog.Logger
	depGraph  *model.DependencyGraph
}

// OrchestratorConfig holds configuration for creating an Orchestrator.
type OrchestratorConfig struct {
	Registry   *injection.Registry
	Observer   observer.Observer
	Reconciler observer.ReconciliationCheckerInterface
	Evaluator  *evaluator.Evaluator
	Lock       safety.ExperimentLock
	Knowledge  *model.OperatorKnowledge
	K8sClient  client.Client
	ReportDir  string
	Verbose  bool
	DepGraph *model.DependencyGraph
	Logger   *slog.Logger
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
	logger := config.Logger
	if logger == nil {
		if config.Verbose {
			logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
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
		k8sClient:  config.K8sClient,
		reportDir:  config.ReportDir,
		verbose:    config.Verbose,
		logger:     logger,
		depGraph:   config.DepGraph,
	}
}

// resolveNamespace returns the experiment namespace, falling back to DefaultNamespace.
func resolveNamespace(exp *v1alpha1.ChaosExperiment) string {
	if exp.Namespace != "" {
		return exp.Namespace
	}
	return v1alpha1.DefaultNamespace
}

// isClusterScopedInjection returns true if the injection type operates on
// cluster-scoped resources where namespace constraints don't apply.
func isClusterScopedInjection(injectionType v1alpha1.InjectionType, params map[string]string) bool {
	switch injectionType {
	case v1alpha1.RBACRevoke:
		// ClusterRoleBinding is cluster-scoped; RoleBinding is namespace-scoped
		return params["bindingType"] == "ClusterRoleBinding"
	case v1alpha1.WebhookDisrupt:
		// ValidatingWebhookConfiguration is always cluster-scoped
		return true
	case v1alpha1.CRDMutation:
		// CRD definitions (apiextensions.k8s.io) are cluster-scoped, but CR
		// instances are typically namespace-scoped.  Default to namespace-scoped
		// so that blast radius checks are enforced for normal CR mutations.
		// Also treat known cluster-scoped kinds from core API groups as cluster-scoped.
		if strings.HasPrefix(params["apiVersion"], "apiextensions.k8s.io/") ||
			strings.HasPrefix(params["apiVersion"], "apiregistration.k8s.io/") {
			return true
		}
		return clusterScopedKinds[params["kind"]]
	case v1alpha1.FinalizerBlock:
		// Cluster-scoped kinds (Namespace, Node, ClusterRole, etc.) require
		// cluster-scoped treatment since they have no namespace.
		// Also check API group prefix for CRD/APIService resources.
		if strings.HasPrefix(params["apiVersion"], "apiextensions.k8s.io/") ||
			strings.HasPrefix(params["apiVersion"], "apiregistration.k8s.io/") {
			return true
		}
		return clusterScopedKinds[params["kind"]]
	default:
		return false
	}
}

// clusterScopedKinds lists Kubernetes resource kinds that are cluster-scoped.
var clusterScopedKinds = map[string]bool{
	"Namespace":                           true,
	"Node":                                true,
	"ClusterRole":                         true,
	"ClusterRoleBinding":                  true,
	"CustomResourceDefinition":            true,
	"PersistentVolume":                    true,
	"StorageClass":                        true,
	"IngressClass":                        true,
	"PriorityClass":                       true,
	"ValidatingWebhookConfiguration":      true,
	"MutatingWebhookConfiguration":        true,
	"APIService":                          true,
}

// forbiddenNamespaces is the set of namespaces that must never be targeted by
// chaos experiments.
var forbiddenNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
	"default":         true,
}

// forbiddenNamespacePrefixes lists namespace prefixes that are always forbidden.
var forbiddenNamespacePrefixes = []string{"openshift-"}

// checkForbiddenNamespace returns an error if the namespace is in the forbidden
// list or matches a forbidden prefix.
func checkForbiddenNamespace(ns, context string) error {
	if forbiddenNamespaces[ns] {
		return fmt.Errorf("%s namespace %q is forbidden", context, ns)
	}
	for _, prefix := range forbiddenNamespacePrefixes {
		if strings.HasPrefix(ns, prefix) {
			return fmt.Errorf("%s namespace %q matches forbidden prefix %q", context, ns, prefix)
		}
	}
	return nil
}

// ValidateExperiment checks blast radius, danger level, injector lookup, and
// injection spec validation.  It does NOT check dry-run or acquire a lock.
func (o *Orchestrator) ValidateExperiment(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	namespace := resolveNamespace(exp)

	// Validate target spec fields are valid Kubernetes names.
	if err := injection.ValidateTargetSpec(exp.Spec.Target); err != nil {
		return fmt.Errorf("target validation failed: %w", err)
	}

	// Determine target resource for forbidden-resource validation
	targetResource := exp.Spec.Target.Resource
	if targetResource == "" {
		targetResource = fmt.Sprintf("%s/%s", exp.Spec.Target.Component, exp.Name)
	}

	// Check resolved namespace against forbidden list.
	if err := checkForbiddenNamespace(namespace, "experiment target"); err != nil {
		return err
	}

	// Reject forbidden namespaces in AllowedNamespaces
	for _, ns := range exp.Spec.BlastRadius.AllowedNamespaces {
		if err := checkForbiddenNamespace(ns, "blast radius"); err != nil {
			return err
		}
	}

	// Validate steady-state check namespaces.
	for _, check := range exp.Spec.SteadyState.Checks {
		if check.Namespace != "" {
			if err := checkForbiddenNamespace(check.Namespace, "steady-state check"); err != nil {
				return err
			}
		}
	}

	// Validate steady-state check discriminated union fields.
	for i, check := range exp.Spec.SteadyState.Checks {
		switch check.Type {
		case v1alpha1.CheckConditionTrue:
			if check.Kind == "" || check.Name == "" || check.ConditionType == "" {
				return fmt.Errorf("steadyState.checks[%d]: type 'conditionTrue' requires kind, name, and conditionType", i)
			}
		case v1alpha1.CheckResourceExists:
			if check.Kind == "" || check.Name == "" {
				return fmt.Errorf("steadyState.checks[%d]: type 'resourceExists' requires kind and name", i)
			}
		case "":
			return fmt.Errorf("steadyState.checks[%d]: type is required", i)
		default:
			return fmt.Errorf("steadyState.checks[%d]: unknown type %q", i, check.Type)
		}
	}

	// Enforce maximum recovery timeout.
	if exp.Spec.Hypothesis.RecoveryTimeout.Duration > v1alpha1.MaxRecoveryTimeout {
		return fmt.Errorf("recoveryTimeout %v exceeds maximum allowed %v", exp.Spec.Hypothesis.RecoveryTimeout.Duration, v1alpha1.MaxRecoveryTimeout)
	}

	// Enforce maximum injection TTL.
	if exp.Spec.Injection.TTL.Duration > v1alpha1.MaxInjectionTTL {
		return fmt.Errorf("injection TTL %v exceeds maximum allowed %v", exp.Spec.Injection.TTL.Duration, v1alpha1.MaxInjectionTTL)
	}

	// Cluster-scoped injections skip namespace-based blast radius validation
	// because they operate on cluster-wide resources (webhooks, CRDs, ClusterRoleBindings).
	clusterScoped := isClusterScopedInjection(exp.Spec.Injection.Type, exp.Spec.Injection.Parameters)
	if clusterScoped {
		if exp.Spec.Injection.DangerLevel != v1alpha1.DangerLevelHigh {
			return fmt.Errorf("cluster-scoped injection type %s requires dangerLevel: high to acknowledge cluster-wide impact", exp.Spec.Injection.Type)
		}
		if len(exp.Spec.BlastRadius.AllowedNamespaces) > 0 {
			return fmt.Errorf("cluster-scoped injection type %s ignores AllowedNamespaces; blast radius cannot be namespace-scoped for cluster-scoped resources", exp.Spec.Injection.Type)
		}
	} else {
		// Namespace-scoped injections require at least one AllowedNamespace.
		if len(exp.Spec.BlastRadius.AllowedNamespaces) == 0 {
			return fmt.Errorf("namespace-scoped injection type %s requires at least one AllowedNamespaces entry", exp.Spec.Injection.Type)
		}
		// Check blast radius (namespace + count + forbidden resources)
		if err := safety.ValidateBlastRadius(exp.Spec.BlastRadius, namespace, targetResource, exp.Spec.Injection.Count); err != nil {
			return fmt.Errorf("blast radius validation failed: %w", err)
		}
	}

	// Check danger level
	if err := safety.CheckDangerLevel(exp.Spec.Injection.DangerLevel, exp.Spec.BlastRadius.AllowDangerous); err != nil {
		return fmt.Errorf("danger level check failed: %w", err)
	}

	// Get injector
	injector, err := o.registry.Get(exp.Spec.Injection.Type)
	if err != nil {
		return fmt.Errorf("unknown injection type: %w", err)
	}

	// Validate injection spec
	if err := injector.Validate(exp.Spec.Injection, exp.Spec.BlastRadius); err != nil {
		return fmt.Errorf("injection validation failed: %w", err)
	}

	return nil
}

// RunPreCheck executes the steady-state pre-check. If no checks are defined it
// returns a passing result.
func (o *Orchestrator) RunPreCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, error) {
	namespace := resolveNamespace(exp)

	if len(exp.Spec.SteadyState.Checks) > 0 {
		return o.observer.CheckSteadyState(ctx, exp.Spec.SteadyState.Checks, namespace)
	}
	return &v1alpha1.CheckResult{Passed: true, Timestamp: metav1.Now()}, nil
}

// InjectFault looks up the injector from the registry and calls Inject.
func (o *Orchestrator) InjectFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) (injection.CleanupFunc, []v1alpha1.InjectionEvent, error) {
	namespace := resolveNamespace(exp)

	injector, err := o.registry.Get(exp.Spec.Injection.Type)
	if err != nil {
		return nil, nil, fmt.Errorf("unknown injection type: %w", err)
	}

	return injector.Inject(ctx, exp.Spec.Injection, namespace)
}

// RevertFault looks up the injector from the registry and calls Revert to undo
// a previously injected fault.
func (o *Orchestrator) RevertFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) error {
	namespace := resolveNamespace(exp)

	injector, err := o.registry.Get(exp.Spec.Injection.Type)
	if err != nil {
		return fmt.Errorf("unknown injection type: %w", err)
	}

	return injector.Revert(ctx, exp.Spec.Injection, namespace)
}

// RunPostCheck executes the observation board pattern: reconciliation
// contributor (phase 1), then steady-state and collateral contributors
// (phase 2, concurrent). It returns the post-check result and the full
// findings list for evaluation.
func (o *Orchestrator) RunPostCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, []observer.Finding, error) {
	namespace := resolveNamespace(exp)
	recoveryTimeout := exp.ResolvedRecoveryTimeout()

	board := observer.NewObservationBoard()

	// Phase 1: Reconciliation (blocking)
	if o.knowledge != nil {
		component := o.knowledge.GetComponent(exp.Spec.Target.Component)
		if component != nil && o.reconciler != nil {
			reconContributor := observer.NewReconciliationContributor(o.reconciler, component, namespace, recoveryTimeout)
			if reconErr := reconContributor.Observe(ctx, board); reconErr != nil {
				o.logger.Warn("reconciliation contributor error", "error", reconErr)
			}
		}
	}

	// Phase 2: Steady-state + collateral (concurrent)
	var phase2Contributors []observer.ObservationContributor

	if len(exp.Spec.SteadyState.Checks) > 0 {
		phase2Contributors = append(phase2Contributors, observer.NewSteadyStateContributor(
			o.observer, exp.Spec.SteadyState.Checks, namespace))
	} else {
		// No checks defined — write a "passed" finding to preserve existing behavior
		board.AddFinding(observer.Finding{
			Source: observer.SourceSteadyState,
			Passed: true,
			Checks: &v1alpha1.CheckResult{Passed: true, Timestamp: metav1.Now()},
		})
	}

	if o.depGraph != nil {
		ref := model.ComponentRef{
			Operator:  exp.Spec.Target.Operator,
			Component: exp.Spec.Target.Component,
		}
		dependents := o.depGraph.DirectDependents(ref)
		if len(dependents) > 0 {
			phase2Contributors = append(phase2Contributors, observer.NewCollateralContributor(
				o.observer, dependents))
		}
	}

	if len(phase2Contributors) > 0 {
		if errs := observer.RunContributors(ctx, board, phase2Contributors); len(errs) > 0 {
			for _, e := range errs {
				o.logger.Warn("phase 2 contributor error", "error", e)
			}
		}
	}

	// Extract post-check result from board
	var postCheck *v1alpha1.CheckResult
	for _, f := range board.FindingsBySource(observer.SourceSteadyState) {
		postCheck = f.Checks
	}
	if postCheck == nil {
		postCheck = &v1alpha1.CheckResult{Passed: true, Timestamp: metav1.Now()}
	}

	return postCheck, board.Findings(), nil
}

// EvaluateExperiment wraps the evaluator's EvaluateFromFindings call.
func (o *Orchestrator) EvaluateExperiment(findings []observer.Finding, hypothesis v1alpha1.HypothesisSpec) *evaluator.EvaluationResult {
	return o.evaluator.EvaluateFromFindings(findings, hypothesis)
}

// Run executes the full experiment lifecycle for the given ChaosExperiment.
// It proceeds through the phases: Pending -> SteadyStatePre -> Injecting ->
// Observing -> SteadyStatePost -> Evaluating -> Complete (or Aborted on error).
func (o *Orchestrator) Run(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*ExperimentResult, error) {
	result := &ExperimentResult{
		Experiment: exp.Name,
		Phase:      v1alpha1.PhasePending,
	}

	// 1. Validate
	o.logger.Info("phase transition", "phase", "PENDING", "experiment", exp.Name, "action", "validating")

	namespace := resolveNamespace(exp)
	if exp.Namespace == "" {
		o.logger.Warn("no namespace specified, using default", "namespace", namespace)
	}

	if err := o.ValidateExperiment(ctx, exp); err != nil {
		result.Error = err.Error()
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("validation failed: %w", err)
	}

	// Dry run check — skip lock acquisition since no faults are injected
	if exp.Spec.BlastRadius.DryRun {
		o.logger.Info("dry run", "injection", exp.Spec.Injection.Type, "operator", exp.Spec.Target.Operator, "component", exp.Spec.Target.Component)
		result.Phase = v1alpha1.PhaseComplete
		result.Verdict = v1alpha1.Inconclusive
		return result, nil
	}

	// Compute recovery timeout early — needed for lease duration calculation.
	recoveryTimeout := exp.ResolvedRecoveryTimeout()

	// Acquire experiment lock with 2x recovery timeout, matching controller behavior.
	leaseDuration := recoveryTimeout * 2
	if err := o.lock.Acquire(ctx, exp.Spec.Target.Operator, exp.Name, leaseDuration); err != nil {
		result.Error = fmt.Sprintf("lock acquisition failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("lock: %w", err)
	}
	defer func() {
		if err := o.lock.Release(context.Background(), exp.Spec.Target.Operator, exp.Name); err != nil {
			o.logger.Warn("lock release failed", "error", err)
		}
	}()

	// 2. Steady State Pre-Check
	o.logger.Info("phase transition", "phase", "STEADY_STATE_PRE", "action", "checking baseline")
	result.Phase = v1alpha1.PhaseSteadyStatePre

	preCheck, preCheckErr := o.RunPreCheck(ctx, exp)
	if preCheckErr != nil {
		result.Error = fmt.Sprintf("steady state pre-check error: %v", preCheckErr)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("pre-check: %w", preCheckErr)
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

	cleanup, events, err := o.InjectFault(ctx, exp)
	if err != nil {
		result.Error = fmt.Sprintf("injection failed: %v", err)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("injection: %w", err)
	}

	// Ensure cleanup runs even if the parent context is cancelled
	defer func() {
		if cleanup != nil {
			o.logger.Info("phase transition", "phase", "CLEANUP")
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), defaultCleanupTimeout)
			defer cleanupCancel()
			if cleanErr := cleanup(cleanupCtx); cleanErr != nil {
				o.logger.Warn("cleanup warning", "error", cleanErr)
				result.CleanupError = cleanErr.Error()
				if result.Report != nil {
					result.Report.CleanupError = cleanErr.Error()
				}
			}
		}
	}()

	o.logger.Info("injection complete", "events", len(events))

	// 4. Observe — wait for recovery timeout before post-check.
	o.logger.Info("phase transition", "phase", "OBSERVING", "action", "waiting for recovery")
	result.Phase = v1alpha1.PhaseObserving

	o.logger.Info("OBSERVING", "recoveryTimeout", recoveryTimeout)
	select {
	case <-time.After(recoveryTimeout):
	case <-ctx.Done():
		// Context cancelled during observation — abort cleanly rather than
		// passing a dead context to RunPostCheck which would produce confusing errors.
		o.logger.Warn("context cancelled during observation, aborting experiment")
		result.Phase = v1alpha1.PhaseAborted
		result.Error = "experiment interrupted: context cancelled during observation"
		return result, ctx.Err()
	}

	// 5. Steady State Post-Check
	o.logger.Info("phase transition", "phase", "STEADY_STATE_POST", "action", "verifying recovery")
	result.Phase = v1alpha1.PhaseSteadyStatePost

	postCheck, findings, postCheckErr := o.RunPostCheck(ctx, exp)
	if postCheckErr != nil {
		o.logger.Warn("post-check error", "error", postCheckErr)
		result.Error = fmt.Sprintf("post-check error: %v", postCheckErr)
		result.Phase = v1alpha1.PhaseAborted
		return result, fmt.Errorf("post-check: %w", postCheckErr)
	}

	// 6. Evaluate
	o.logger.Info("phase transition", "phase", "EVALUATING")
	result.Phase = v1alpha1.PhaseEvaluating

	evalResult := o.EvaluateExperiment(findings, exp.Spec.Hypothesis)
	result.Evaluation = evalResult
	result.Verdict = evalResult.Verdict

	// Extract reconciliation data from findings for report
	var reconciliationResult *observer.ReconciliationResult
	for _, f := range findings {
		if f.Source == observer.SourceReconciliation {
			reconciliationResult = f.ReconciliationResult
		}
	}

	// 7. Report

	// Extract injection targets from events
	var injectionTargets []string
	for _, ev := range events {
		if ev.Target != "" {
			injectionTargets = append(injectionTargets, ev.Target)
		}
	}

	// Build collateral findings for report
	var collateralFindings []reporter.CollateralFinding
	for _, f := range findings {
		if f.Source == observer.SourceCollateral {
			collateralFindings = append(collateralFindings, reporter.CollateralFinding{
				Operator:  f.Operator,
				Component: f.Component,
				Passed:    f.Passed,
				Checks:    f.Checks,
			})
		}
	}

	report := reporter.ExperimentReport{
		Experiment: exp.Name,
		Timestamp:  time.Now(),
		Target: reporter.TargetReport{
			Operator:  exp.Spec.Target.Operator,
			Component: exp.Spec.Target.Component,
			Resource:  exp.Spec.Target.Resource,
		},
		Injection: reporter.InjectionReport{
			Type:      string(exp.Spec.Injection.Type),
			Targets:   injectionTargets,
			Timestamp: time.Now(),
		},
		SteadyState: reporter.SteadyStateReport{
			Pre:  preCheck,
			Post: postCheck,
		},
		Evaluation:     *evalResult,
		Reconciliation: reconciliationResult,
		Collateral:     collateralFindings,
	}
	result.Report = &report

	// Write JSON report if reportDir specified
	if o.reportDir != "" {
		reportPath := filepath.Join(o.reportDir, fmt.Sprintf("%s-%s.json", exp.Name, time.Now().Format("20060102-150405")))
		r, err := reporter.NewJSONFileReporter(reportPath)
		if err != nil {
			o.logger.Warn("creating report file", "path", reportPath, "error", err)
		} else {
			if writeErr := r.Write(report); writeErr != nil {
				o.logger.Warn("writing report", "path", reportPath, "error", writeErr)
			}
			if closeErr := r.Close(); closeErr != nil {
				o.logger.Warn("closing report file", "path", reportPath, "error", closeErr)
			}
		}
	}

	// Store result as ConfigMap in cluster
	if o.k8sClient != nil {
		o.storeResultConfigMap(ctx, exp, namespace, report)
	}

	o.logger.Info("phase transition", "phase", "COMPLETE", "verdict", evalResult.Verdict)
	result.Phase = v1alpha1.PhaseComplete

	return result, nil
}

// configMapNameMaxLen is the maximum length for a Kubernetes resource name.
const configMapNameMaxLen = 253

// labelValueMaxLen is the maximum length for a Kubernetes label value.
const labelValueMaxLen = 63

// truncateLabel truncates a string to the maximum Kubernetes label value length.
func truncateLabel(s string) string {
	if len(s) <= labelValueMaxLen {
		return s
	}
	return s[:labelValueMaxLen]
}

// storeResultConfigMap creates a ConfigMap in the experiment's namespace
// containing the JSON-serialized ExperimentReport, making results visible
// via kubectl get configmap -l app.kubernetes.io/managed-by=odh-chaos.
func (o *Orchestrator) storeResultConfigMap(ctx context.Context, exp *v1alpha1.ChaosExperiment, namespace string, report reporter.ExperimentReport) {
	// Use a dedicated context so that ConfigMap storage succeeds even if the
	// parent context is near its deadline.
	storeCtx, storeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer storeCancel()

	reportJSON, err := json.Marshal(report)
	if err != nil {
		o.logger.Warn("marshaling report for ConfigMap", "error", err)
		return
	}

	cmName := "chaos-result-" + exp.Name
	if len(cmName) > configMapNameMaxLen {
		cmName = cmName[:configMapNameMaxLen]
	}
	// Ensure the name does not end with a non-alphanumeric character
	cmName = strings.TrimRight(cmName, "-._")

	cmLabels := map[string]string{
		"app.kubernetes.io/managed-by":    "odh-chaos",
		"chaos.opendatahub.io/experiment": truncateLabel(exp.Name),
		"chaos.opendatahub.io/verdict":    strings.ToLower(string(report.Evaluation.Verdict)),
	}
	cmAnnotations := map[string]string{
		"chaos.opendatahub.io/timestamp": report.Timestamp.UTC().Format(time.RFC3339),
	}
	cmData := map[string]string{
		"result.json": string(reportJSON),
	}

	existing := &corev1.ConfigMap{}
	existing.Name = cmName
	existing.Namespace = namespace
	result, err := controllerutil.CreateOrUpdate(storeCtx, o.k8sClient, existing, func() error {
		existing.Labels = cmLabels
		existing.Annotations = cmAnnotations
		existing.Data = cmData
		return nil
	})
	if err != nil {
		o.logger.Warn("storing result ConfigMap", "name", cmName, "namespace", namespace, "error", err)
	} else {
		o.logger.Info("result ConfigMap stored", "name", cmName, "namespace", namespace, "operation", result)
	}
}


package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/evaluator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/injection"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/orchestrator"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	cleanupFinalizer      = "chaos.opendatahub.io/cleanup"
	immediateRequeue      = 100 * time.Millisecond
	lockContentionRequeue = 30 * time.Second
	maxObserveRequeue     = 30 * time.Second
)


// Compile-time interface check: *orchestrator.Orchestrator implements PhaseOrchestrator.
var _ PhaseOrchestrator = (*orchestrator.Orchestrator)(nil)

// PhaseOrchestrator defines the interface for experiment lifecycle operations.
type PhaseOrchestrator interface {
	ValidateExperiment(ctx context.Context, exp *v1alpha1.ChaosExperiment) error
	RunPreCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, error)
	InjectFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) (injection.CleanupFunc, []v1alpha1.InjectionEvent, error)
	RevertFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) error
	RunPostCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, []observer.Finding, error)
	EvaluateExperiment(findings []observer.Finding, hypothesis v1alpha1.HypothesisSpec) *evaluator.EvaluationResult
}

// ChaosExperimentReconciler reconciles ChaosExperiment CRs using the
// phase-per-reconcile pattern: each reconcile loop advances the experiment
// by exactly one phase.
//
// RBAC permissions are intentionally broad to support dynamic experiment targets.
// Safety is enforced via application-layer validation (pkg/injection/validate.go)
// using deny-lists for system-critical resources. Cluster admins should review
// ChaosExperiment CRs before approval.
//
//+kubebuilder:rbac:groups=chaos.opendatahub.io,resources=chaosexperiments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=chaos.opendatahub.io,resources=chaosexperiments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=chaos.opendatahub.io,resources=chaosexperiments/finalizers,verbs=update
//+kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;delete
//+kubebuilder:rbac:groups="",resources=configmaps;secrets,verbs=get;list;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces;serviceaccounts,verbs=get;list
//+kubebuilder:rbac:groups=apps,resources=deployments;replicasets;statefulsets,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;create;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings;roles;rolebindings,verbs=get;list;update;patch
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations;mutatingwebhookconfigurations,verbs=get;list;update;patch
type ChaosExperimentReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Orchestrator PhaseOrchestrator
	Lock         safety.ExperimentLock
	Clock        clock.Clock
	Recorder     record.EventRecorder
}

// Reconcile is the main reconcile loop for ChaosExperiment CRs.
func (r *ChaosExperimentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("experiment", req.NamespacedName)

	exp := &v1alpha1.ChaosExperiment{}
	if err := r.Get(ctx, req.NamespacedName, exp); err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger = logger.WithValues("phase", exp.Status.Phase)

	// Deletion check — FIRST, before any phase-based early returns.
	if !exp.DeletionTimestamp.IsZero() {
		logger.Info("handling deletion")
		return r.handleDeletion(ctx, exp)
	}

	// Clean up finalizer if still present on terminal experiments.
	// This handles the case where the controller crashed between status
	// update and finalizer removal.
	if exp.Status.Phase == v1alpha1.PhaseComplete || exp.Status.Phase == v1alpha1.PhaseAborted {
		if controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
			// Best-effort revert: if the controller crashed between setting
			// Phase=Aborted and calling RevertFault, the fault may still be
			// active. RevertFault is idempotent — calling it when already
			// reverted is a no-op.
			if exp.Status.Phase == v1alpha1.PhaseAborted {
				if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
					logger.Error(err, "best-effort revert on terminal cleanup failed")
				}
			}
			controllerutil.RemoveFinalizer(exp, cleanupFinalizer)
			if err := r.Update(ctx, exp); err != nil {
				return ctrl.Result{}, err
			}
			// Release lease best-effort on terminal cleanup (handles crash between
			// abort status update and lease release).
			if err := r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
				log.FromContext(ctx).Error(err, "best-effort lock release on terminal cleanup failed")
			}
		}

		// Backfill status fields for Aborted experiments that may have empty
		// Message/EndTime if the controller crashed between the abort crash
		// barrier and the full status update. This runs regardless of finalizer
		// presence, since abort() from PhaseInjecting may crash before the
		// finalizer is added.
		if exp.Status.Phase == v1alpha1.PhaseAborted {
			statusChanged := false
			if exp.Status.Message == "" {
				exp.Status.Message = "experiment aborted (recovered after crash)"
				statusChanged = true
			}
			if exp.Status.EndTime == nil {
				now := metav1.NewTime(r.Clock.Now())
				exp.Status.EndTime = &now
				statusChanged = true
			}
			// Clear InjectionStartedAt if still present (crash between abort
			// crash barrier and full status update in abort()).
			if exp.Status.InjectionStartedAt != nil {
				exp.Status.InjectionStartedAt = nil
				statusChanged = true
			}
			// Backfill ConditionComplete if missing (crash between abort crash
			// barrier and condition-setting code).
			hasCompleteCondition := false
			for _, c := range exp.Status.Conditions {
				if c.Type == v1alpha1.ConditionComplete {
					hasCompleteCondition = true
					break
				}
			}
			if !hasCompleteCondition {
				setCondition(exp, r.Clock.Now(), v1alpha1.ConditionComplete, metav1.ConditionFalse, "ExperimentAborted", "recovered after crash")
				statusChanged = true
			}
			if statusChanged {
				if err := r.Status().Update(ctx, exp); err != nil {
					return ctrl.Result{}, fmt.Errorf("status backfill on terminal cleanup: %w", err)
				}
			}
		}

		return ctrl.Result{}, nil
	}

	// Spec mutation detection.
	if exp.Status.ObservedGeneration != 0 && exp.Generation != exp.Status.ObservedGeneration {
		r.Recorder.Event(exp, "Warning", "SpecMutated", "Spec changed during active experiment")
		return r.abort(ctx, exp, "spec mutated after experiment started")
	}

	// Set ObservedGeneration in-memory. This is only persisted when a phase
	// handler's status update succeeds, which satisfies the K8s convention
	// that ObservedGeneration reflects the generation the controller acted on.
	exp.Status.ObservedGeneration = exp.Generation

	// Lease renewal — if past Pending phase.
	if exp.Status.Phase != v1alpha1.PhasePending && exp.Status.Phase != "" {
		if err := r.Lock.Renew(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
			// Abort if the lease was taken by another experiment (holder mismatch)
			// or if the lock no longer exists (not found).
			if errors.Is(err, safety.ErrHolderMismatch) || errors.Is(err, safety.ErrLockNotFound) {
				return r.abort(ctx, exp, fmt.Sprintf("lease renewal failed: %v", err))
			}
			// For transient errors (network, etc.), requeue to retry.
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// Phase switch — dispatch to phase handler.
	switch exp.Status.Phase {
	case "", v1alpha1.PhasePending:
		return r.reconcilePending(ctx, exp)
	case v1alpha1.PhaseSteadyStatePre:
		return r.reconcilePreCheck(ctx, exp)
	case v1alpha1.PhaseInjecting:
		return r.reconcileInject(ctx, exp)
	case v1alpha1.PhaseObserving:
		return r.reconcileObserve(ctx, exp)
	case v1alpha1.PhaseSteadyStatePost:
		return r.reconcilePostCheck(ctx, exp)
	case v1alpha1.PhaseEvaluating:
		return r.reconcileEvaluate(ctx, exp)
	default:
		return r.abort(ctx, exp, fmt.Sprintf("unknown phase %q", exp.Status.Phase))
	}
}

// reconcilePending validates the experiment, acquires the lock, sets StartTime,
// and transitions to SteadyStatePre.
func (r *ChaosExperimentReconciler) reconcilePending(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	if err := r.Orchestrator.ValidateExperiment(ctx, exp); err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("validation failed: %v", err))
	}

	recoveryTimeout := exp.ResolvedRecoveryTimeout()
	leaseDuration := recoveryTimeout * 2

	if err := r.Lock.Acquire(ctx, exp.Spec.Target.Operator, exp.Name, leaseDuration); err != nil {
		if errors.Is(err, safety.ErrLockContention) {
			r.Recorder.Event(exp, "Normal", "LockContention", fmt.Sprintf("Lock held by another experiment, requeuing: %v", err))
			return ctrl.Result{RequeueAfter: lockContentionRequeue}, nil
		}
		return ctrl.Result{}, err
	}

	log.FromContext(ctx).Info("experiment started", "operator", exp.Spec.Target.Operator, "injection", exp.Spec.Injection.Type)

	now := metav1.NewTime(r.Clock.Now())
	exp.Status.StartTime = &now
	exp.Status.Phase = v1alpha1.PhaseSteadyStatePre
	exp.Status.Message = fmt.Sprintf("Running pre-checks for %s", exp.Spec.Target.Operator)
	r.Recorder.Event(exp, "Normal", "ExperimentStarted", "Experiment started, transitioning to SteadyStatePre")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// reconcilePreCheck runs the pre-check, stores results, and transitions to Injecting.
// If the pre-check fails, the experiment is aborted.
func (r *ChaosExperimentReconciler) reconcilePreCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	result, err := r.Orchestrator.RunPreCheck(ctx, exp)
	if err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("pre-check error: %v", err))
	}

	exp.Status.SteadyStatePre = result
	r.Recorder.Event(exp, "Normal", "PreCheckComplete", fmt.Sprintf("Pre-check passed=%t", result.Passed))

	if !result.Passed {
		return r.abort(ctx, exp, "pre-check failed: steady-state not established")
	}

	setCondition(exp, r.Clock.Now(), v1alpha1.ConditionSteadyStateEstablished, metav1.ConditionTrue, "PreCheckPassed", "Steady-state established before injection")
	exp.Status.Phase = v1alpha1.PhaseInjecting
	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// reconcileInject performs fault injection. It adds the cleanup finalizer,
// calls InjectFault, stores injection events, and transitions to Observing.
// Re-entry guard: if InjectionStartedAt is already set, skip to Observing.
func (r *ChaosExperimentReconciler) reconcileInject(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	// Re-entry guard: if InjectionStartedAt was persisted (crash barrier) but
	// the controller crashed before completing injection, skip to Observing.
	if exp.Status.InjectionStartedAt != nil {
		// Ensure finalizer is present — it may be missing if the controller
		// crashed between the crash barrier persist and the finalizer add.
		if !controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
			controllerutil.AddFinalizer(exp, cleanupFinalizer)
			if err := r.Update(ctx, exp); err != nil {
				return ctrl.Result{}, err
			}
		}
		exp.Status.Phase = v1alpha1.PhaseObserving
		if err := r.Status().Update(ctx, exp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: immediateRequeue}, nil
	}

	// Persist InjectionStartedAt BEFORE calling InjectFault as a crash barrier.
	// If the controller crashes after InjectFault but before status update,
	// the re-entry guard (line above) will skip re-injection on the next reconcile.
	//
	// NOTE: If the controller crashes AFTER this persist but BEFORE InjectFault,
	// the re-entry guard will skip injection even though no fault was injected.
	// The experiment will proceed through Observing -> PostCheck -> Evaluate and
	// report "Resilient" (since the system was never disrupted). This is a safe
	// false-positive outcome in a very narrow crash window.
	now := metav1.NewTime(r.Clock.Now())
	exp.Status.InjectionStartedAt = &now
	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	// Add finalizer AFTER crash barrier is persisted. If the controller crashes
	// between the crash barrier and here, the re-entry guard will skip re-injection
	// and no finalizer will be orphaned.
	if !controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
		controllerutil.AddFinalizer(exp, cleanupFinalizer)
		if err := r.Update(ctx, exp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// NOTE: CleanupFunc is intentionally discarded. The controller is stateless
	// across reconciles so closures cannot survive restarts. All injectors persist
	// rollback data in annotations/Secrets and implement idempotent Revert methods.
	// The CleanupFunc is only used by the standalone orchestrator (pkg/orchestrator/lifecycle.go).
	_, events, err := r.Orchestrator.InjectFault(ctx, exp)
	if err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("injection failed: %v", err))
	}

	// InjectionLog is best-effort: if this status update fails after successful
	// injection, the re-entry guard will skip re-injection and InjectionLog
	// will be empty. This is acceptable — the injection itself succeeded.
	log.FromContext(ctx).Info("fault injected", "type", exp.Spec.Injection.Type, "events", len(events))

	exp.Status.InjectionLog = events
	setCondition(exp, r.Clock.Now(), v1alpha1.ConditionFaultInjected, metav1.ConditionTrue, "FaultInjected", "Fault successfully injected")
	exp.Status.Phase = v1alpha1.PhaseObserving
	exp.Status.Message = fmt.Sprintf("Fault injected, observing recovery for %s", exp.Spec.Hypothesis.RecoveryTimeout.Duration)
	r.Recorder.Event(exp, "Normal", "FaultInjected", "Fault injected, transitioning to Observing")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// reconcileObserve waits for the recovery timeout to elapse, then reverts the
// fault and transitions to SteadyStatePost.
func (r *ChaosExperimentReconciler) reconcileObserve(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	if exp.Status.InjectionStartedAt == nil {
		return r.abort(ctx, exp, "InjectionStartedAt is nil in Observing phase")
	}

	elapsed := r.Clock.Now().Sub(exp.Status.InjectionStartedAt.Time)

	// TTL enforcement: if TTL is set and exceeded, force-revert immediately
	// regardless of recovery timeout. This prevents injections from running
	// longer than their declared TTL.
	if exp.Spec.Injection.TTL.Duration > 0 && elapsed > exp.Spec.Injection.TTL.Duration {
		log.FromContext(ctx).Info("injection TTL exceeded, force-reverting", "ttl", exp.Spec.Injection.TTL.Duration, "elapsed", elapsed)
		r.Recorder.Event(exp, "Warning", "TTLExceeded", fmt.Sprintf("Injection TTL %s exceeded after %s", exp.Spec.Injection.TTL.Duration, elapsed))
	} else {
		recoveryTimeout := exp.ResolvedRecoveryTimeout()
		if elapsed < recoveryTimeout {
			remaining := recoveryTimeout - elapsed
			requeue := remaining
			if requeue > maxObserveRequeue {
				requeue = maxObserveRequeue
			}
			return ctrl.Result{RequeueAfter: requeue}, nil
		}
	}

	// Verify lease is still held before reverting — if the lease expired and
	// another experiment acquired it, we must not revert their injection.
	// NOTE: This is an intentional second Renew call per reconcile (the first
	// is the per-reconcile renewal in Reconcile()). The cost of the extra API
	// call is justified by the safety guarantee: we confirm the lease is still
	// ours immediately before reverting, closing the window where the lease
	// could expire between the top-of-loop renewal and the revert.
	if err := r.Lock.Renew(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("pre-revert lease check failed: %v", err))
	}

	// Recovery timeout elapsed — revert the fault.
	// NOTE: RevertFault implementations MUST be idempotent. If the controller
	// crashes after RevertFault succeeds but before the status update below,
	// RevertFault will run again on re-entry. This is safe because revert
	// restores from rollback data, which is a no-op if already restored.
	if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("post-timeout revert failed: %v", err))
	}

	exp.Status.Phase = v1alpha1.PhaseSteadyStatePost
	r.Recorder.Event(exp, "Normal", "FaultReverted", "Fault reverted, transitioning to SteadyStatePost")

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// reconcilePostCheck runs the post-check, stores results, and evaluates the
// hypothesis in a single reconcile to avoid inconsistency between the stored
// SteadyStatePost result and the evaluation verdict.
func (r *ChaosExperimentReconciler) reconcilePostCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	result, findings, err := r.Orchestrator.RunPostCheck(ctx, exp)
	if err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("post-check error: %v", err))
	}

	exp.Status.SteadyStatePost = result
	condStatus := metav1.ConditionTrue
	if !result.Passed {
		condStatus = metav1.ConditionFalse
	}
	setCondition(exp, r.Clock.Now(), v1alpha1.ConditionRecoveryObserved, condStatus, "PostCheckComplete", fmt.Sprintf("Post-check passed=%t", result.Passed))
	exp.Status.Phase = v1alpha1.PhaseEvaluating
	r.Recorder.Event(exp, "Normal", "PostCheckComplete", fmt.Sprintf("Post-check passed=%t", result.Passed))

	// Evaluate inline using the same findings to guarantee consistency
	// between the stored post-check result and the verdict.
	return r.doEvaluate(ctx, exp, findings)
}

// reconcileEvaluate handles crash recovery: if the controller restarted while
// in Evaluating phase, re-run RunPostCheck to obtain findings for evaluation.
func (r *ChaosExperimentReconciler) reconcileEvaluate(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	_, findings, err := r.Orchestrator.RunPostCheck(ctx, exp)
	if err != nil {
		return r.abort(ctx, exp, fmt.Sprintf("evaluation post-check error: %v", err))
	}
	return r.doEvaluate(ctx, exp, findings)
}

// doEvaluate performs the actual evaluation, sets verdict, EndTime, persists
// terminal status BEFORE releasing lock. Finalizer removal is deferred to
// the next reconcile (the Complete/Aborted cleanup block at the top of
// Reconcile handles this), avoiding the race where finalizer removal before
// status update could orphan the lease.
func (r *ChaosExperimentReconciler) doEvaluate(ctx context.Context, exp *v1alpha1.ChaosExperiment, findings []observer.Finding) (ctrl.Result, error) {
	evalResult := r.Orchestrator.EvaluateExperiment(findings, exp.Spec.Hypothesis)

	exp.Status.Verdict = evalResult.Verdict
	exp.Status.EvaluationResult = toEvaluationSummary(evalResult)
	now := metav1.NewTime(r.Clock.Now())
	exp.Status.EndTime = &now
	exp.Status.Message = fmt.Sprintf("Experiment complete, verdict: %s", evalResult.Verdict)
	setCondition(exp, r.Clock.Now(), v1alpha1.ConditionComplete, metav1.ConditionTrue, "ExperimentComplete", fmt.Sprintf("Verdict: %s", evalResult.Verdict))
	exp.Status.Phase = v1alpha1.PhaseComplete
	r.Recorder.Event(exp, "Normal", "ExperimentComplete", fmt.Sprintf("Experiment complete, verdict: %s", evalResult.Verdict))

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	// Release lock AFTER status is persisted (best effort).
	if err := r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
		log.FromContext(ctx).Error(err, "best-effort lock release after completion failed")
	}

	// Finalizer will be removed on next reconcile.
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// handleDeletion handles the deletion of a ChaosExperiment CR.
// It reverts any active fault, releases the lock, and removes the finalizer.
func (r *ChaosExperimentReconciler) handleDeletion(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
		return ctrl.Result{}, nil
	}

	// Revert fault — return error to retry if revert fails, ensuring the
	// finalizer prevents deletion until the fault is cleaned up.
	if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
		exp.Status.CleanupError = fmt.Sprintf("deletion revert failed: %v", err)
		r.Recorder.Event(exp, "Warning", "CleanupError", fmt.Sprintf("Failed to revert fault during deletion: %v", err))
		_ = r.Status().Update(ctx, exp)
		return ctrl.Result{}, fmt.Errorf("reverting fault during deletion: %w", err)
	}

	// Release lock (best effort).
	if err := r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
		log.FromContext(ctx).Error(err, "best-effort lock release during deletion failed")
	}

	// Remove finalizer — only after successful revert.
	controllerutil.RemoveFinalizer(exp, cleanupFinalizer)
	if err := r.Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// abort handles aborting an experiment. If the experiment has an active fault
// (Injecting, Observing, or SteadyStatePost), it reverts it first.
// Terminal status is persisted BEFORE releasing the lock. Finalizer removal
// is deferred to the next reconcile to avoid the race where deletion during
// the window between finalizer removal and status update orphans the lease.
func (r *ChaosExperimentReconciler) abort(ctx context.Context, exp *v1alpha1.ChaosExperiment, reason string) (ctrl.Result, error) {
	// Revert fault if in an active-fault phase.
	var revertError string
	switch exp.Status.Phase {
	case v1alpha1.PhaseInjecting:
		// Clear InjectionStartedAt and set Phase=Aborted as a crash barrier.
		// This runs whether or not InjectFault succeeded: if InjectFault failed,
		// InjectionStartedAt is still persisted from the pre-injection crash barrier.
		// Clearing it here prevents the re-entry guard from incorrectly skipping
		// to Observing on retry if this abort's final status update fails.
		exp.Status.InjectionStartedAt = nil
		exp.Status.Phase = v1alpha1.PhaseAborted
		if err := r.Status().Update(ctx, exp); err != nil {
			return ctrl.Result{}, err
		}
		// Best-effort revert: if InjectFault partially applied before failing,
		// RevertFault is idempotent and will clean up. If InjectFault never ran,
		// RevertFault is a no-op (no rollback annotations to process).
		if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
			revertError = fmt.Sprintf("abort revert failed: %v", err)
		}
	case v1alpha1.PhaseObserving, v1alpha1.PhaseSteadyStatePost:
		if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
			revertError = fmt.Sprintf("abort revert failed: %v", err)
		}
	}

	log.FromContext(ctx).Info("aborting experiment", "reason", reason, "revertError", revertError)

	exp.Status.Phase = v1alpha1.PhaseAborted
	exp.Status.CleanupError = revertError
	exp.Status.Message = reason
	now := metav1.NewTime(r.Clock.Now())
	exp.Status.EndTime = &now
	setCondition(exp, r.Clock.Now(), v1alpha1.ConditionComplete, metav1.ConditionFalse, "ExperimentAborted", reason)
	r.Recorder.Event(exp, "Warning", "ExperimentAborted", reason)

	if err := r.Status().Update(ctx, exp); err != nil {
		return ctrl.Result{}, err
	}

	// Release lock AFTER status is persisted (best effort).
	if err := r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
		log.FromContext(ctx).Error(err, "best-effort lock release after abort failed")
	}

	// Finalizer will be removed on next reconcile.
	return ctrl.Result{RequeueAfter: immediateRequeue}, nil
}

// SetupWithManager registers the reconciler with the controller manager.
// GenerationChangedPredicate filters out status-only updates, reducing unnecessary
// reconciles for terminal experiments. AnnotationChangedPredicate catches
// annotation changes (e.g., crash barrier annotations from injectors); note that
// finalizer changes are in metadata.finalizers (not annotations) and are handled
// by RequeueAfter from phase handlers. LabelChangedPredicate catches label
// changes that would otherwise be missed.
func (r *ChaosExperimentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ChaosExperiment{}, builder.WithPredicates(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
				predicate.LabelChangedPredicate{},
			),
		)).
		Complete(r)
}

// toEvaluationSummary converts an evaluator.EvaluationResult to a CRD-embeddable EvaluationSummary.
func toEvaluationSummary(result *evaluator.EvaluationResult) *v1alpha1.EvaluationSummary {
	if result == nil {
		return nil
	}
	deviations := make([]string, 0, len(result.Deviations))
	for _, d := range result.Deviations {
		deviations = append(deviations, fmt.Sprintf("%s: %s", d.Type, d.Detail))
	}
	return &v1alpha1.EvaluationSummary{
		Verdict:         result.Verdict,
		Confidence:      result.Confidence,
		RecoveryTime:    result.RecoveryTime.String(),
		ReconcileCycles: result.ReconcileCycles,
		Deviations:      deviations,
	}
}

// setCondition sets or updates a condition on the experiment status.
func setCondition(exp *v1alpha1.ChaosExperiment, now time.Time, condType string, status metav1.ConditionStatus, reason, message string) {
	transition := metav1.NewTime(now)
	for i, c := range exp.Status.Conditions {
		if c.Type == condType {
			// Only update LastTransitionTime when the status actually changes,
			// per K8s condition conventions.
			if c.Status != status {
				exp.Status.Conditions[i].LastTransitionTime = transition
			}
			exp.Status.Conditions[i].Status = status
			exp.Status.Conditions[i].Reason = reason
			exp.Status.Conditions[i].Message = message
			exp.Status.Conditions[i].ObservedGeneration = exp.Generation
			return
		}
	}
	exp.Status.Conditions = append(exp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: transition,
		ObservedGeneration: exp.Generation,
	})
}

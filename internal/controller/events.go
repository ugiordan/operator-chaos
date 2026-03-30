package controller

import (
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

// Event reason constants following K8s CamelCase conventions.
const (
	ReasonExperimentStarted  = "ExperimentStarted"
	ReasonPhaseTransition    = "PhaseTransition"
	ReasonPreCheckComplete   = "PreCheckComplete"
	ReasonPreCheckFailed     = "PreCheckFailed"
	ReasonFaultInjected      = "FaultInjected"
	ReasonInjectionFailed    = "InjectionFailed"
	ReasonFaultReverted      = "FaultReverted"
	ReasonRevertFailed       = "RevertFailed"
	ReasonPostCheckComplete  = "PostCheckComplete"
	ReasonPostCheckFailed    = "PostCheckFailed"
	ReasonObserverFindings   = "ObserverFindings"
	ReasonRecoveryDetected   = "RecoveryDetected"
	ReasonVerdictReached     = "VerdictReached"
	ReasonExperimentComplete = "ExperimentComplete"
	ReasonExperimentAborted  = "ExperimentAborted"
	ReasonTTLExceeded        = "TTLExceeded"
	ReasonLockContention     = "LockContention"
	ReasonLeaseRenewalFailed = "LeaseRenewalFailed"
	ReasonSpecMutated        = "SpecMutated"
	ReasonCleanupError       = "CleanupError"
)

// EventEmitter emits structured K8s events for a ChaosExperiment.
// Created once per reconcile, scoped to a single experiment.
type EventEmitter struct {
	recorder record.EventRecorder
	exp      *v1alpha1.ChaosExperiment
}

// NewEventEmitter creates an EventEmitter for the given experiment.
func NewEventEmitter(recorder record.EventRecorder, exp *v1alpha1.ChaosExperiment) *EventEmitter {
	return &EventEmitter{recorder: recorder, exp: exp}
}

func (e *EventEmitter) ExperimentStarted() {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonExperimentStarted, "Experiment started, transitioning to SteadyStatePre")
}

func (e *EventEmitter) PhaseTransition(from, to v1alpha1.ExperimentPhase) {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonPhaseTransition, fmt.Sprintf("Phase transition: %s -> %s", from, to))
}

func (e *EventEmitter) PreCheckComplete(passed bool) {
	if passed {
		e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonPreCheckComplete, fmt.Sprintf("Pre-check passed=%t", passed))
	} else {
		e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonPreCheckFailed, fmt.Sprintf("Pre-check passed=%t", passed))
	}
}

func (e *EventEmitter) PostCheckComplete(passed bool) {
	if passed {
		e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonPostCheckComplete, fmt.Sprintf("Post-check passed=%t", passed))
	} else {
		e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonPostCheckFailed, fmt.Sprintf("Post-check passed=%t", passed))
	}
}

func (e *EventEmitter) FaultInjected() {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonFaultInjected, "Fault injected, transitioning to Observing")
}

func (e *EventEmitter) InjectionFailed(err error) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonInjectionFailed, fmt.Sprintf("Fault injection failed: %v", err))
}

func (e *EventEmitter) FaultReverted() {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonFaultReverted, "Fault reverted, transitioning to SteadyStatePost")
}

func (e *EventEmitter) RevertFailed(err error) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonRevertFailed, fmt.Sprintf("Fault revert failed: %v", err))
}

func (e *EventEmitter) LeaseRenewalFailed(err error) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonLeaseRenewalFailed, fmt.Sprintf("Lease renewal failed: %v", err))
}

func (e *EventEmitter) VerdictReached(verdict v1alpha1.Verdict) {
	eventType := corev1.EventTypeNormal
	if verdict != v1alpha1.Resilient {
		eventType = corev1.EventTypeWarning
	}
	e.recorder.Event(e.exp, eventType, ReasonVerdictReached, fmt.Sprintf("Verdict: %s", verdict))
}

func (e *EventEmitter) ExperimentComplete(verdict v1alpha1.Verdict) {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonExperimentComplete, fmt.Sprintf("Experiment complete, verdict: %s", verdict))
}

func (e *EventEmitter) ObserverFindings(count int, summary string) {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonObserverFindings, fmt.Sprintf("Observer: %d findings: %s", count, summary))
}

func (e *EventEmitter) RecoveryDetected(recoveryTime string, reconcileCycles int) {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonRecoveryDetected, fmt.Sprintf("Recovery detected in %s (%d reconcile cycles)", recoveryTime, reconcileCycles))
}

func (e *EventEmitter) ExperimentAborted(reason string) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonExperimentAborted, reason)
}

func (e *EventEmitter) TTLExceeded(ttl, elapsed string) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonTTLExceeded, fmt.Sprintf("Injection TTL %s exceeded after %s", ttl, elapsed))
}

func (e *EventEmitter) LockContention(detail string) {
	e.recorder.Event(e.exp, corev1.EventTypeNormal, ReasonLockContention, fmt.Sprintf("Lock held by another experiment, requeuing: %s", detail))
}

func (e *EventEmitter) SpecMutated() {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonSpecMutated, "Spec changed during active experiment")
}

func (e *EventEmitter) CleanupError(err error) {
	e.recorder.Event(e.exp, corev1.EventTypeWarning, ReasonCleanupError, fmt.Sprintf("Failed to revert fault during deletion: %v", err))
}

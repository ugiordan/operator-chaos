package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

func newTestEmitter() (*EventEmitter, *record.FakeRecorder) {
	rec := record.NewFakeRecorder(100)
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exp",
			Namespace: "test-ns",
		},
	}
	return NewEventEmitter(rec, exp), rec
}

func TestExperimentStarted(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.ExperimentStarted()

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonExperimentStarted)
	assert.Contains(t, event, "Experiment started")
}

func TestPhaseTransition(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.PhaseTransition(v1alpha1.PhaseSteadyStatePre, v1alpha1.PhaseInjecting)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonPhaseTransition)
	assert.Contains(t, event, "SteadyStatePre")
	assert.Contains(t, event, "Injecting")
}

func TestPreCheckComplete_Passed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.PreCheckComplete(true)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonPreCheckComplete)
	assert.Contains(t, event, "passed=true")
}

func TestPreCheckComplete_Failed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.PreCheckComplete(false)

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonPreCheckFailed)
	assert.Contains(t, event, "passed=false")
}

func TestPostCheckComplete_Passed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.PostCheckComplete(true)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonPostCheckComplete)
}

func TestPostCheckComplete_Failed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.PostCheckComplete(false)

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonPostCheckFailed)
}

func TestInjectionFailed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.InjectionFailed(fmt.Errorf("connection refused"))

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonInjectionFailed)
	assert.Contains(t, event, "connection refused")
}

func TestRevertFailed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.RevertFailed(fmt.Errorf("timeout"))

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonRevertFailed)
	assert.Contains(t, event, "timeout")
}

func TestFaultInjected(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.FaultInjected()

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonFaultInjected)
}

func TestFaultReverted(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.FaultReverted()

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonFaultReverted)
}

func TestLeaseRenewalFailed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.LeaseRenewalFailed(fmt.Errorf("holder mismatch"))

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonLeaseRenewalFailed)
	assert.Contains(t, event, "holder mismatch")
}

func TestVerdictReached_Resilient(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.VerdictReached(v1alpha1.Resilient)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonVerdictReached)
	assert.Contains(t, event, "Resilient")
}

func TestVerdictReached_Failed(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.VerdictReached(v1alpha1.Failed)

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonVerdictReached)
	assert.Contains(t, event, "Failed")
}

func TestVerdictReached_Degraded(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.VerdictReached(v1alpha1.Degraded)

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonVerdictReached)
	assert.Contains(t, event, "Degraded")
}

func TestVerdictReached_Inconclusive(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.VerdictReached(v1alpha1.Inconclusive)

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonVerdictReached)
	assert.Contains(t, event, "Inconclusive")
}

func TestExperimentComplete(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.ExperimentComplete(v1alpha1.Resilient)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonExperimentComplete)
	assert.Contains(t, event, "Resilient")
}

func TestObserverFindings(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.ObserverFindings(3, "2 passed, 1 failed")

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonObserverFindings)
	assert.Contains(t, event, "3 findings")
}

func TestRecoveryDetected(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.RecoveryDetected("1.2s", 5)

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonRecoveryDetected)
	assert.Contains(t, event, "1.2s")
	assert.Contains(t, event, "5 reconcile cycles")
}

func TestExperimentAborted(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.ExperimentAborted("pre-check failed")

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonExperimentAborted)
	assert.Contains(t, event, "pre-check failed")
}

func TestTTLExceeded(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.TTLExceeded("30s", "45s")

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonTTLExceeded)
	assert.Contains(t, event, "30s")
	assert.Contains(t, event, "45s")
}

func TestLockContention(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.LockContention("held by other-exp")

	event := <-rec.Events
	assert.Contains(t, event, "Normal")
	assert.Contains(t, event, ReasonLockContention)
	assert.Contains(t, event, "held by other-exp")
}

func TestSpecMutated(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.SpecMutated()

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonSpecMutated)
}

func TestCleanupError(t *testing.T) {
	emitter, rec := newTestEmitter()
	emitter.CleanupError(fmt.Errorf("revert timeout"))

	event := <-rec.Events
	assert.Contains(t, event, "Warning")
	assert.Contains(t, event, ReasonCleanupError)
	assert.Contains(t, event, "revert timeout")
}

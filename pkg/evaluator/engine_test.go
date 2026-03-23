package evaluator

import (
	"testing"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEvaluateResilient(t *testing.T) {
	e := New(10)

	result := e.Evaluate(
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		true, // all reconciled
		2,    // reconcile cycles
		12*time.Second, // recovery time
		v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}},
	)

	assert.Equal(t, v1alpha1.Resilient, result.Verdict)
	assert.Equal(t, 12*time.Second, result.RecoveryTime)
	assert.Equal(t, 2, result.ReconcileCycles)
	assert.NotEmpty(t, result.Confidence)
}

func TestEvaluateFailed(t *testing.T) {
	e := New(10)

	result := e.Evaluate(
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		&v1alpha1.CheckResult{Passed: false, ChecksRun: 3, ChecksPassed: 1},
		false, 0, 120*time.Second,
		v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}},
	)

	assert.Equal(t, v1alpha1.Failed, result.Verdict)
}

func TestEvaluateDegraded_SlowRecovery(t *testing.T) {
	e := New(10)

	result := e.Evaluate(
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		true, 3, 90*time.Second,
		v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}},
	)

	assert.Equal(t, v1alpha1.Degraded, result.Verdict)
}

func TestEvaluateDegraded_ExcessiveCycles(t *testing.T) {
	e := New(5) // max 5 cycles

	result := e.Evaluate(
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		&v1alpha1.CheckResult{Passed: true, ChecksRun: 3, ChecksPassed: 3},
		true, 15, 30*time.Second, // 15 cycles > max 5
		v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}},
	)

	assert.Equal(t, v1alpha1.Degraded, result.Verdict)
	assert.NotEmpty(t, result.Deviations)
}

func TestEvaluateInconclusive(t *testing.T) {
	e := New(10)

	result := e.Evaluate(
		&v1alpha1.CheckResult{Passed: false, ChecksRun: 3, ChecksPassed: 1}, // pre-check failed
		&v1alpha1.CheckResult{Passed: false, ChecksRun: 3, ChecksPassed: 1},
		false, 0, 0,
		v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}},
	)

	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
}

// --- EvaluateFromFindings tests ---

func defaultHypothesis() v1alpha1.HypothesisSpec {
	return v1alpha1.HypothesisSpec{RecoveryTimeout: metav1.Duration{Duration: 60 * time.Second}}
}

func reconFinding(allReconciled bool, cycles int, recovery time.Duration) observer.Finding {
	return observer.Finding{
		Source: observer.SourceReconciliation,
		ReconciliationResult: &observer.ReconciliationResult{
			AllReconciled:   allReconciled,
			ReconcileCycles: cycles,
			RecoveryTime:    recovery,
		},
	}
}

func ssFinding(passed bool, run, passed_ int32) observer.Finding {
	return observer.Finding{
		Source: observer.SourceSteadyState,
		Checks: &v1alpha1.CheckResult{Passed: passed, ChecksRun: run, ChecksPassed: passed_},
	}
}

func collateralFinding(passed bool, operator, component string) observer.Finding {
	return observer.Finding{
		Source:    observer.SourceCollateral,
		Passed:   passed,
		Operator: operator,
		Component: component,
	}
}

func TestEvaluateFromFindings_ResilientNoCollateral(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(true, 2, 10*time.Second),
		ssFinding(true, 3, 3),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Resilient, result.Verdict)
	assert.Empty(t, result.Deviations)
}

func TestEvaluateFromFindings_ResilientDowngradedByCollateral(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(true, 2, 10*time.Second),
		ssFinding(true, 3, 3),
		collateralFinding(false, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Degraded, result.Verdict)
	assert.Len(t, result.Deviations, 1)
	assert.Equal(t, "collateral_degradation", result.Deviations[0].Type)
}

func TestEvaluateFromFindings_FailedNotDowngradedByCollateral(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(false, 0, 120*time.Second),
		ssFinding(false, 3, 1),
		collateralFinding(false, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Failed, result.Verdict)
}

func TestEvaluateFromFindings_InconclusiveNotDowngradedByCollateral(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		collateralFinding(false, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Inconclusive, result.Verdict)
	assert.Equal(t, "no steady-state post-check data", result.Confidence)
}

func TestEvaluateFromFindings_CollateralAllPassNoDowngrade(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(true, 2, 10*time.Second),
		ssFinding(true, 3, 3),
		collateralFinding(true, "opA", "compA"),
		collateralFinding(true, "opB", "compB"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Resilient, result.Verdict)
	assert.Empty(t, result.Deviations)
}

func TestEvaluateFromFindings_DegradedNotChangedByCollateralPass(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(false, 2, 10*time.Second),
		ssFinding(true, 3, 3),
		collateralFinding(true, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Degraded, result.Verdict)
}

func TestEvaluateFromFindings_DegradedNotChangedByCollateralFail(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(false, 2, 10*time.Second),
		ssFinding(true, 3, 3),
		collateralFinding(false, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Degraded, result.Verdict)
}

func TestEvaluateFromFindings_FailedNotChangedByCollateralPass(t *testing.T) {
	e := New(10)
	findings := []observer.Finding{
		reconFinding(true, 2, 10*time.Second),
		ssFinding(false, 3, 1),
		collateralFinding(true, "opA", "compA"),
	}
	result := e.EvaluateFromFindings(findings, defaultHypothesis())
	assert.Equal(t, v1alpha1.Failed, result.Verdict)
}

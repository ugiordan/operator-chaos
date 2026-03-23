package evaluator

import (
	"fmt"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/observer"
)

// Evaluator classifies experiment outcomes into verdicts based on
// steady-state checks, reconciliation status, recovery time, and
// reconcile cycle counts.
type Evaluator struct {
	maxReconcileCycles int
}

// New creates an Evaluator that flags excessive reconciliation when cycles exceed maxReconcileCycles.
func New(maxReconcileCycles int) *Evaluator {
	return &Evaluator{maxReconcileCycles: maxReconcileCycles}
}

// Evaluate compares pre- and post-injection check results against the hypothesis and returns a verdict.
func (e *Evaluator) Evaluate(
	preCheck *v1alpha1.CheckResult,
	postCheck *v1alpha1.CheckResult,
	allReconciled bool,
	reconcileCycles int,
	recoveryTime time.Duration,
	hypothesis v1alpha1.HypothesisSpec,
) *EvaluationResult {
	result := &EvaluationResult{
		RecoveryTime:    recoveryTime,
		ReconcileCycles: reconcileCycles,
	}

	if preCheck == nil || postCheck == nil {
		result.Verdict = v1alpha1.Inconclusive
		result.Confidence = "missing pre-check or post-check data"
		return result
	}

	if !preCheck.Passed {
		result.Verdict = v1alpha1.Inconclusive
		result.Confidence = fmt.Sprintf(
			"pre-check failed: %d/%d checks passed",
			preCheck.ChecksPassed, preCheck.ChecksRun)
		return result
	}

	result.Verdict, result.Deviations = e.computeVerdict(postCheck, allReconciled, reconcileCycles, recoveryTime, hypothesis)
	result.Confidence = fmt.Sprintf(
		"%d/%d steady-state checks passed, %s recovery, %d reconcile cycles",
		postCheck.ChecksPassed, postCheck.ChecksRun, recoveryTime, reconcileCycles)

	return result
}

// EvaluateFromFindings produces a verdict from observer findings, including collateral damage analysis.
func (e *Evaluator) EvaluateFromFindings(
	findings []observer.Finding,
	hypothesis v1alpha1.HypothesisSpec,
) *EvaluationResult {
	allReconciled := true
	reconcileCycles := 0
	recoveryTime := time.Duration(0)

	for _, f := range findings {
		if f.Source == observer.SourceReconciliation && f.ReconciliationResult != nil {
			allReconciled = f.ReconciliationResult.AllReconciled
			reconcileCycles = f.ReconciliationResult.ReconcileCycles
			recoveryTime = f.ReconciliationResult.RecoveryTime
			break
		}
	}

	var postCheck *v1alpha1.CheckResult
	for _, f := range findings {
		if f.Source == observer.SourceSteadyState {
			postCheck = f.Checks
			break
		}
	}

	result := &EvaluationResult{
		RecoveryTime:    recoveryTime,
		ReconcileCycles: reconcileCycles,
	}

	if postCheck == nil {
		result.Verdict = v1alpha1.Inconclusive
		result.Confidence = "no steady-state post-check data"
		return result
	}

	result.Verdict, result.Deviations = e.computeVerdict(postCheck, allReconciled, reconcileCycles, recoveryTime, hypothesis)
	result.Confidence = fmt.Sprintf(
		"%d/%d steady-state checks passed, %s recovery, %d reconcile cycles",
		postCheck.ChecksPassed, postCheck.ChecksRun, recoveryTime, reconcileCycles)

	// Collateral downgrade: only Resilient → Degraded
	for _, f := range findings {
		if f.Source == observer.SourceCollateral && !f.Passed {
			if result.Verdict == v1alpha1.Resilient {
				result.Verdict = v1alpha1.Degraded
			}
			result.Deviations = append(result.Deviations, Deviation{
				Type:   "collateral_degradation",
				Detail: fmt.Sprintf("dependent %s/%s degraded", f.Operator, f.Component),
			})
		}
	}

	return result
}

func (e *Evaluator) computeVerdict(
	postCheck *v1alpha1.CheckResult,
	allReconciled bool,
	reconcileCycles int,
	recoveryTime time.Duration,
	hypothesis v1alpha1.HypothesisSpec,
) (v1alpha1.Verdict, []Deviation) {
	var verdict v1alpha1.Verdict
	var deviations []Deviation

	if postCheck.Passed && allReconciled {
		verdict = v1alpha1.Resilient
	} else if postCheck.Passed && !allReconciled {
		verdict = v1alpha1.Degraded
		deviations = append(deviations, Deviation{
			Type:   "partial_reconciliation",
			Detail: "steady state checks passed but not all resources reconciled",
		})
	} else {
		verdict = v1alpha1.Failed
	}

	if hypothesis.RecoveryTimeout.Duration > 0 && recoveryTime > hypothesis.RecoveryTimeout.Duration {
		if verdict == v1alpha1.Resilient {
			verdict = v1alpha1.Degraded
		}
		deviations = append(deviations, Deviation{
			Type: "slow_recovery",
			Detail: fmt.Sprintf("recovered in %s, expected within %s",
				recoveryTime, hypothesis.RecoveryTimeout.Duration),
		})
	}

	if e.maxReconcileCycles > 0 && reconcileCycles > e.maxReconcileCycles {
		if verdict == v1alpha1.Resilient {
			verdict = v1alpha1.Degraded
		}
		deviations = append(deviations, Deviation{
			Type: "excessive_reconciliation",
			Detail: fmt.Sprintf("%d cycles (max %d)",
				reconcileCycles, e.maxReconcileCycles),
		})
	}

	return verdict, deviations
}

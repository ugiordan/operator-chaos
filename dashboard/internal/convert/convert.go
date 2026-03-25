package convert

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
)

const (
	labelSuiteName       = "chaos.opendatahub.io/suite-name"
	labelSuiteRunID      = "chaos.opendatahub.io/suite-run-id"
	labelOperatorVersion = "chaos.opendatahub.io/operator-version"
)

func FromCR(cr *v1alpha1.ChaosExperiment) (*store.Experiment, error) {
	specJSON, err := json.Marshal(cr.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshaling spec: %w", err)
	}
	statusJSON, err := json.Marshal(cr.Status)
	if err != nil {
		return nil, fmt.Errorf("marshaling status: %w", err)
	}

	startTime := cr.CreationTimestamp.Time
	if cr.Status.StartTime != nil {
		startTime = cr.Status.StartTime.Time
	}

	id := fmt.Sprintf("%s/%s/%s", cr.Namespace, cr.Name, startTime.Format(time.RFC3339))

	exp := &store.Experiment{
		ID:              id,
		Name:            cr.Name,
		Namespace:       cr.Namespace,
		Operator:        cr.Spec.Target.Operator,
		Component:       cr.Spec.Target.Component,
		InjectionType:   string(cr.Spec.Injection.Type),
		Phase:           string(cr.Status.Phase),
		Verdict:         string(cr.Status.Verdict),
		DangerLevel:     string(cr.Spec.Injection.DangerLevel),
		CleanupError:    cr.Status.CleanupError,
		SpecJSON:        string(specJSON),
		StatusJSON:      string(statusJSON),
		StartTime:       &startTime,
		SuiteName:       cr.Labels[labelSuiteName],
		SuiteRunID:      cr.Labels[labelSuiteRunID],
		OperatorVersion: cr.Labels[labelOperatorVersion],
	}

	if cr.Status.EndTime != nil {
		t := cr.Status.EndTime.Time
		exp.EndTime = &t
	}

	if cr.Status.EvaluationResult != nil && cr.Status.EvaluationResult.RecoveryTime != "" {
		d, err := time.ParseDuration(cr.Status.EvaluationResult.RecoveryTime)
		if err == nil {
			ms := d.Milliseconds()
			exp.RecoveryMs = &ms
		}
	}

	return exp, nil
}

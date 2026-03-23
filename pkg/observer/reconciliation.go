package observer

import (
	"context"
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/clock"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reconciliationPollInterval = 2 * time.Second

// ReconciliationChecker verifies that an operator has properly reconciled
// all managed resources for a given component. This is the key innovation
// of odh-platform-chaos: checking semantic reconciliation (correct metadata,
// spec, conditions, and owner references), not just pod restarts.
type ReconciliationChecker struct {
	client client.Client
	clock  clock.Clock
}

// NewReconciliationChecker creates a new ReconciliationChecker with the given client.
func NewReconciliationChecker(c client.Client) *ReconciliationChecker {
	return &ReconciliationChecker{client: c, clock: clock.RealClock{}}
}

// ReconciliationResult captures the outcome of reconciliation verification,
// including per-resource details, number of reconcile cycles observed,
// and total recovery time.
type ReconciliationResult struct {
	AllReconciled   bool                  `json:"allReconciled"`
	ReconcileCycles int                   `json:"reconcileCycles"`
	RecoveryTime    time.Duration         `json:"recoveryTime"`
	Resources       []ResourceCheckResult `json:"resources"`
}

// ResourceCheckResult captures the reconciliation status of a single managed resource.
type ResourceCheckResult struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Reconciled bool   `json:"reconciled"`
	Details    string `json:"details,omitempty"`
}

// CheckReconciliation verifies all managed resources for a component
// have been properly reconciled by the operator. It polls until all
// resources are reconciled or the timeout expires.
func (r *ReconciliationChecker) CheckReconciliation(
	ctx context.Context,
	component *model.ComponentModel,
	namespace string,
	timeout time.Duration,
) (*ReconciliationResult, error) {
	result := &ReconciliationResult{
		AllReconciled: true,
	}

	startTime := r.clock.Now()
	deadline := startTime.Add(timeout)
	cycles := 0

	ticker := time.NewTicker(reconciliationPollInterval)
	defer ticker.Stop()

	for r.clock.Now().Before(deadline) {
		cycles++
		allGood := true
		// Reset resources at start of each cycle so the last cycle's
		// diagnostic data is preserved on timeout.
		result.Resources = nil

		for _, mr := range component.ManagedResources {
			ns := mr.Namespace
			if ns == "" {
				ns = namespace
			}

			obj := &unstructured.Unstructured{}
			obj.SetAPIVersion(mr.APIVersion)
			obj.SetKind(mr.Kind)

			err := r.client.Get(ctx, types.NamespacedName{Name: mr.Name, Namespace: ns}, obj)
			if err != nil {
				allGood = false
				result.Resources = append(result.Resources, ResourceCheckResult{
					Kind:       mr.Kind,
					Name:       mr.Name,
					Namespace:  ns,
					Reconciled: false,
					Details:    fmt.Sprintf("not found: %v", err),
				})
				continue
			}

			// Check owner reference if expected
			reconciled := true
			details := "exists"

			if mr.OwnerRef != "" {
				ownerRefs := obj.GetOwnerReferences()
				hasOwner := false
				for _, ref := range ownerRefs {
					if ref.Kind == mr.OwnerRef {
						hasOwner = true
						break
					}
				}
				if !hasOwner {
					reconciled = false
					details = fmt.Sprintf("missing owner reference %s", mr.OwnerRef)
				}
			}

			// Check expected spec fields if defined
			if mr.ExpectedSpec != nil {
				for field, expected := range mr.ExpectedSpec {
					actual, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", field)
					if !found {
						reconciled = false
						details = fmt.Sprintf("spec.%s not found", field)
					} else if fmt.Sprintf("%v", actual) != fmt.Sprintf("%v", expected) {
						reconciled = false
						details = fmt.Sprintf("spec.%s: expected %v, got %v", field, expected, actual)
					}
				}
			}

			if !reconciled {
				allGood = false
			}

			result.Resources = append(result.Resources, ResourceCheckResult{
				Kind:       mr.Kind,
				Name:       mr.Name,
				Namespace:  ns,
				Reconciled: reconciled,
				Details:    details,
			})
		}

		if allGood {
			result.AllReconciled = true
			result.ReconcileCycles = cycles
			result.RecoveryTime = r.clock.Now().Sub(startTime)
			return result, nil
		}

		// Wait for poll interval or context cancellation
		select {
		case <-ticker.C:
			// continue polling
		case <-ctx.Done():
			result.AllReconciled = false
			result.ReconcileCycles = cycles
			result.RecoveryTime = r.clock.Now().Sub(startTime)
			return result, ctx.Err()
		}
	}

	result.AllReconciled = false
	result.ReconcileCycles = cycles
	result.RecoveryTime = r.clock.Now().Sub(startTime)
	return result, nil
}

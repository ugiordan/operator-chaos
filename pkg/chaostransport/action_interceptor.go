package chaostransport

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// ActionFaultConfig defines fault injection for a specific action in the reconciler pipeline.
type ActionFaultConfig struct {
	// Skip makes the action a no-op (returns nil without running).
	Skip bool
	// FailBefore returns an error before the action runs.
	FailBefore string
	// FailAfter runs the action, then returns an error (simulates partial failure).
	FailAfter string
	// Delay adds latency before the action runs.
	Delay time.Duration
	// CorruptField sets a field in the ReconciliationRequest to a wrong value before the action runs.
	// Format: "fieldPath=value" (e.g., "Instance.Spec.Replicas=0")
	CorruptField string
	// ErrorRate is the probability (0.0-1.0) that the fault activates. Default 1.0.
	ErrorRate float64
}

// ActionInterceptor wraps action functions with chaos fault injection.
// It reads fault configuration for each action by matching action names.
type ActionInterceptor struct {
	faults map[string]ActionFaultConfig
}

// NewActionInterceptor creates an interceptor with the given fault configs.
// Keys are action name patterns (matched case-insensitively against the action's
// full qualified name, e.g., "deploy", "gc", "initialize").
func NewActionInterceptor(faults map[string]ActionFaultConfig) *ActionInterceptor {
	normalized := make(map[string]ActionFaultConfig, len(faults))
	for k, v := range faults {
		if v.ErrorRate == 0 {
			v.ErrorRate = 1.0
		}
		normalized[strings.ToLower(k)] = v
	}
	return &ActionInterceptor{faults: normalized}
}

// ActionFn is a generic action function type matching the opendatahub-operator's actions.Fn.
type ActionFn func(ctx context.Context, rr interface{}) error

// Wrap wraps an action function with fault injection.
// The actionName is used to look up fault configuration.
func (ai *ActionInterceptor) Wrap(actionName string, fn ActionFn) ActionFn {
	return func(ctx context.Context, rr interface{}) error {
		fc := ai.matchFault(actionName)
		if fc == nil {
			return fn(ctx, rr)
		}

		if fc.ErrorRate < 1.0 && rand.Float64() > fc.ErrorRate {
			return fn(ctx, rr)
		}

		if fc.Skip {
			return nil
		}

		if fc.Delay > 0 {
			time.Sleep(fc.Delay)
		}

		if fc.FailBefore != "" {
			return &ChaosError{
				Operation: "action",
				Message:       fmt.Sprintf("chaos-action(%s): %s", actionName, fc.FailBefore),
			}
		}

		err := fn(ctx, rr)

		if fc.FailAfter != "" {
			return &ChaosError{
				Operation: "action",
				Message:       fmt.Sprintf("chaos-action(%s): %s", actionName, fc.FailAfter),
			}
		}

		return err
	}
}

func (ai *ActionInterceptor) matchFault(actionName string) *ActionFaultConfig {
	lower := strings.ToLower(actionName)
	for pattern, fc := range ai.faults {
		if strings.Contains(lower, pattern) {
			return &fc
		}
	}
	return nil
}

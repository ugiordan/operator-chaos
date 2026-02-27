package sdk

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Option configures the chaos reconciler wrapper.
type Option func(*chaosReconciler)

// WithFaultConfig sets the fault configuration for the wrapper.
func WithFaultConfig(fc *FaultConfig) Option {
	return func(cr *chaosReconciler) {
		cr.faults = fc
	}
}

type chaosReconciler struct {
	inner  reconcile.Reconciler
	faults *FaultConfig
}

// WrapReconciler wraps a standard reconciler with chaos fault injection.
// Usage: chaos.WrapReconciler(myReconciler, chaos.WithFaultConfig(fc))
func WrapReconciler(inner reconcile.Reconciler, opts ...Option) reconcile.Reconciler {
	cr := &chaosReconciler{inner: inner}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

func (cr *chaosReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	if cr.faults != nil {
		if err := cr.faults.MaybeInject("reconcile"); err != nil {
			return ctrl.Result{}, err
		}
	}
	return cr.inner.Reconcile(ctx, req)
}

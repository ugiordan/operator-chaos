package sdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type fakeReconciler struct {
	called bool
}

func (f *fakeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	f.called = true
	return ctrl.Result{}, nil
}

func TestWrapReconcilerPassthrough(t *testing.T) {
	inner := &fakeReconciler{}
	wrapped := WrapReconciler(inner)
	require.NotNil(t, wrapped)

	result, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	assert.NoError(t, err)
	assert.True(t, inner.called)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestWrapReconcilerWithFaults(t *testing.T) {
	inner := &fakeReconciler{}
	faults := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"reconcile": {ErrorRate: 1.0, Error: "reconcile chaos"},
		},
	}
	wrapped := WrapReconciler(inner, WithFaultConfig(faults))

	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reconcile chaos")
	assert.False(t, inner.called, "inner reconciler should not be called when fault fires")
}

func TestWrapReconcilerInactiveFaults(t *testing.T) {
	inner := &fakeReconciler{}
	faults := &FaultConfig{
		Active: false,
		Faults: map[string]FaultSpec{
			"reconcile": {ErrorRate: 1.0, Error: "should not fire"},
		},
	}
	wrapped := WrapReconciler(inner, WithFaultConfig(faults))

	result, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	assert.NoError(t, err)
	assert.True(t, inner.called)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestWrapReconcilerImplementsInterface(t *testing.T) {
	var _ reconcile.Reconciler = WrapReconciler(&fakeReconciler{})
}

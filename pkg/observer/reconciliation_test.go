package observer

import (
	"context"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewReconciliationChecker(t *testing.T) {
	rc := NewReconciliationChecker(nil)
	assert.NotNil(t, rc)
}

func TestReconciliationResultDefaults(t *testing.T) {
	result := &ReconciliationResult{}
	assert.False(t, result.AllReconciled)
	assert.Equal(t, 0, result.ReconcileCycles)
}

func TestResourceCheckResult(t *testing.T) {
	r := ResourceCheckResult{
		Kind:       "Deployment",
		Name:       "test-deploy",
		Namespace:  "test-ns",
		Reconciled: true,
		Details:    "exists",
	}
	assert.True(t, r.Reconciled)
	assert.Equal(t, "Deployment", r.Kind)
}

func TestCheckReconciliationRespectsContextCancellation(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	rc := NewReconciliationChecker(fakeClient)

	// Create a component with a managed resource that won't exist,
	// so the checker will loop and try to poll again.
	component := &model.ComponentModel{
		Name: "test-component",
		ManagedResources: []model.ManagedResource{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "nonexistent",
				Namespace:  "default",
			},
		},
	}

	// Create a context that is already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	result, err := rc.CheckReconciliation(ctx, component, "default", 30*time.Second)
	elapsed := time.Since(start)

	// Should return quickly (well under the 30s timeout) with a context error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, result.AllReconciled)
	// Should complete in under 1 second since context was already cancelled
	assert.Less(t, elapsed, 1*time.Second, "should return promptly when context is cancelled")
}

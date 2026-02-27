package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNetworkPartitionValidate(t *testing.T) {
	injector := &NetworkPartitionInjector{}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}

	tests := []struct {
		name    string
		spec    v1alpha1.InjectionSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid spec with labelSelector",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.NetworkPartition,
				Parameters: map[string]string{
					"labelSelector": "app=dashboard",
				},
			},
			wantErr: false,
		},
		{
			name: "missing labelSelector",
			spec: v1alpha1.InjectionSpec{
				Type:       v1alpha1.NetworkPartition,
				Parameters: map[string]string{},
			},
			wantErr: true,
			errMsg:  "labelSelector",
		},
		{
			name: "nil parameters",
			spec: v1alpha1.InjectionSpec{
				Type: v1alpha1.NetworkPartition,
			},
			wantErr: true,
			errMsg:  "labelSelector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injector.Validate(tt.spec, blast)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetworkPartitionInjectUsesChaosLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewNetworkPartitionInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.NetworkPartition,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}

	ctx := context.Background()
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
	assert.NotNil(t, cleanup)

	// Fetch the created NetworkPolicy
	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, fakeClient.List(ctx, policies, client.InNamespace("test-ns")))
	require.Len(t, policies.Items, 1, "expected exactly one NetworkPolicy to be created")

	policy := policies.Items[0]

	// Verify labels match safety.ChaosLabels()
	expectedLabels := safety.ChaosLabels(string(v1alpha1.NetworkPartition))
	for k, v := range expectedLabels {
		assert.Equal(t, v, policy.Labels[k], "label %s should match ChaosLabels()", k)
	}

	// Verify specific label values
	assert.Equal(t, safety.ManagedByValue, policy.Labels[safety.ManagedByLabel],
		"managed-by label should use safety constant")
	assert.Equal(t, string(v1alpha1.NetworkPartition), policy.Labels[safety.ChaosTypeLabel],
		"chaos-type label should match injection type")
}

func TestNetworkPartitionInjectAndCleanup(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	injector := NewNetworkPartitionInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type: v1alpha1.NetworkPartition,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}

	ctx := context.Background()

	// Inject: should create a NetworkPolicy
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, v1alpha1.NetworkPartition, events[0].Type)
	assert.Equal(t, "created", events[0].Action)
	require.NotNil(t, cleanup)

	// Verify NetworkPolicy exists
	policies := &networkingv1.NetworkPolicyList{}
	require.NoError(t, fakeClient.List(ctx, policies, client.InNamespace("test-ns")))
	require.Len(t, policies.Items, 1, "expected exactly one NetworkPolicy after inject")

	// Cleanup: should delete the NetworkPolicy
	require.NoError(t, cleanup(ctx))

	// Verify NetworkPolicy is gone
	policies = &networkingv1.NetworkPolicyList{}
	require.NoError(t, fakeClient.List(ctx, policies, client.InNamespace("test-ns")))
	assert.Len(t, policies.Items, 0, "expected no NetworkPolicies after cleanup")
}

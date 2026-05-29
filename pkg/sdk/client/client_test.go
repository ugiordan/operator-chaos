package chaosclient

import (
	"context"
	"testing"

	sdk "github.com/opendatahub-io/operator-chaos/pkg/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestChaosClientPassthrough(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		},
	).Build()

	cc := NewChaosClient(inner, nil)

	cm := &corev1.ConfigMap{}
	err := cc.Get(context.Background(), client.ObjectKey{Name: "test-cm", Namespace: "default"}, cm)
	assert.NoError(t, err)
	assert.Equal(t, "test-cm", cm.Name)
}

func TestChaosClientFaultInjection(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		},
	).Build()

	faults := &sdk.FaultConfig{
		Active: true,
		Faults: map[sdk.Operation]sdk.FaultSpec{
			sdk.OpGet: {ErrorRate: 1.0, Error: "api server error"},
		},
	}

	cc := NewChaosClient(inner, faults)

	cm := &corev1.ConfigMap{}
	err := cc.Get(context.Background(), client.ObjectKey{Name: "test-cm", Namespace: "default"}, cm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api server error")
}

func TestChaosClientImplementsInterface(t *testing.T) {
	// Compile-time check
	var _ client.Client = (*ChaosClient)(nil)
}

func TestChaosClientInactiveFaultsPassthrough(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		},
	).Build()

	faults := &sdk.FaultConfig{
		Active: false,
		Faults: map[sdk.Operation]sdk.FaultSpec{
			sdk.OpGet: {ErrorRate: 1.0, Error: "should not fire"},
		},
	}

	cc := NewChaosClient(inner, faults)

	cm := &corev1.ConfigMap{}
	err := cc.Get(context.Background(), client.ObjectKey{Name: "test-cm", Namespace: "default"}, cm)
	assert.NoError(t, err)
	assert.Equal(t, "test-cm", cm.Name)
}

func TestChaosClientFaultInjectionOnAllOperations(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	tests := []struct {
		name     string
		faultKey sdk.Operation
		errMsg   string
		callFn   func(cc *ChaosClient) error
	}{
		{
			name:     "list fault",
			faultKey: sdk.OpList,
			errMsg:   "list chaos fault",
			callFn: func(cc *ChaosClient) error {
				return cc.List(context.Background(), &corev1.ConfigMapList{})
			},
		},
		{
			name:     "create fault",
			faultKey: sdk.OpCreate,
			errMsg:   "create chaos fault",
			callFn: func(cc *ChaosClient) error {
				return cc.Create(context.Background(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "new-cm", Namespace: "default"},
				})
			},
		},
		{
			name:     "update fault",
			faultKey: sdk.OpUpdate,
			errMsg:   "update chaos fault",
			callFn: func(cc *ChaosClient) error {
				return cc.Update(context.Background(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
				})
			},
		},
		{
			name:     "delete fault",
			faultKey: sdk.OpDelete,
			errMsg:   "delete chaos fault",
			callFn: func(cc *ChaosClient) error {
				return cc.Delete(context.Background(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
				})
			},
		},
		{
			name:     "patch fault",
			faultKey: sdk.OpPatch,
			errMsg:   "patch chaos fault",
			callFn: func(cc *ChaosClient) error {
				return cc.Patch(context.Background(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
				}, client.MergeFrom(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
				},
			).Build()

			faults := &sdk.FaultConfig{
				Active: true,
				Faults: map[sdk.Operation]sdk.FaultSpec{
					tt.faultKey: {ErrorRate: 1.0, Error: tt.errMsg},
				},
			}

			cc := NewChaosClient(inner, faults)
			err := tt.callFn(cc)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

package sdk

import (
	"context"
	"testing"

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

	faults := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 1.0, Error: "api server error"},
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

	faults := &FaultConfig{
		Active: false,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 1.0, Error: "should not fire"},
		},
	}

	cc := NewChaosClient(inner, faults)

	cm := &corev1.ConfigMap{}
	err := cc.Get(context.Background(), client.ObjectKey{Name: "test-cm", Namespace: "default"}, cm)
	assert.NoError(t, err)
	assert.Equal(t, "test-cm", cm.Name)
}

package injection

import (
	"context"
	"testing"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPodKillValidate(t *testing.T) {
	injector := &PodKillInjector{}

	// Valid spec
	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}

	err := injector.Validate(spec, blast)
	assert.NoError(t, err)

	// Invalid: count exceeds blast radius
	spec.Count = 5
	err = injector.Validate(spec, blast)
	assert.Error(t, err)
}

func TestPodKillValidateMissingSelector(t *testing.T) {
	injector := &PodKillInjector{}

	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
	}
	blast := v1alpha1.BlastRadiusSpec{
		MaxPodsAffected:   1,
		AllowedNamespaces: []string{"test"},
	}

	err := injector.Validate(spec, blast)
	assert.Error(t, err)
}

func TestPodKillInjectSuccessful(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "dashboard"},
			UID:       types.UID("uid-1"),
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "dashboard"},
			UID:       types.UID("uid-2"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod1, pod2).
		Build()

	injector := NewPodKillInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}

	ctx := context.Background()
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.NotNil(t, cleanup)

	// Verify only 1 pod remains
	podList := &corev1.PodList{}
	require.NoError(t, fakeClient.List(ctx, podList, client.InNamespace("test-ns")))
	assert.Len(t, podList.Items, 1, "expected 1 pod remaining after killing 1 of 2")
}

func TestPodKillInjectNoPodsFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	injector := NewPodKillInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 1,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}

	ctx := context.Background()
	_, _, err := injector.Inject(ctx, spec, "test-ns")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no pods found")
}

func TestPodKillInjectCountExceedsAvailable(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "test-ns",
			Labels:    map[string]string{"app": "dashboard"},
			UID:       types.UID("uid-1"),
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod1).
		Build()

	injector := NewPodKillInjector(fakeClient)

	spec := v1alpha1.InjectionSpec{
		Type:  v1alpha1.PodKill,
		Count: 5,
		Parameters: map[string]string{
			"labelSelector": "app=dashboard",
		},
	}

	ctx := context.Background()
	cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
	require.NoError(t, err)
	assert.Len(t, events, 1, "should cap to 1 available pod")
	assert.NotNil(t, cleanup)

	// Verify pod was deleted
	podList := &corev1.PodList{}
	require.NoError(t, fakeClient.List(ctx, podList, client.InNamespace("test-ns")))
	assert.Len(t, podList.Items, 0, "expected 0 pods remaining after killing the only available pod")
}

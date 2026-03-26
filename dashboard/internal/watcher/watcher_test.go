package watcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
)

type fakeStore struct {
	upserted []store.Experiment
}

func (f *fakeStore) Upsert(exp store.Experiment) error {
	f.upserted = append(f.upserted, exp)
	return nil
}

func TestHandleCREvent_Upserts(t *testing.T) {
	fs := &fakeStore{}
	w := &Watcher{store: fs, broadcaster: nil}

	now := metav1.Now()
	cr := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-exp",
			Namespace:         "opendatahub",
			CreationTimestamp: now,
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "op", Component: "comp"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase: v1alpha1.PhaseObserving,
		},
	}

	err := w.handleCREvent(cr)
	require.NoError(t, err)
	require.Len(t, fs.upserted, 1)
	assert.Equal(t, "test-exp", fs.upserted[0].Name)
	assert.Equal(t, "Observing", fs.upserted[0].Phase)
}

func TestWatcher_InitialSync(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	now := metav1.Now()
	cr := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "synced",
			Namespace:         "opendatahub",
			CreationTimestamp: now,
		},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "op", Component: "comp"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.ConfigDrift},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhaseComplete, Verdict: v1alpha1.Resilient},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	fs := &fakeStore{}
	w := NewWatcher(client, fs, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := w.SyncOnce(ctx)
	require.NoError(t, err)
	require.Len(t, fs.upserted, 1)
	assert.Equal(t, "synced", fs.upserted[0].Name)
}

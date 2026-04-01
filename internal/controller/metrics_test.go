package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

func TestDescribeSendsAllDescs(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	collector := NewExperimentCollector(c)

	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	var descs []*prometheus.Desc
	for d := range ch {
		descs = append(descs, d)
	}
	assert.Len(t, descs, 6, "expected 6 metric descriptors")
}

func TestCollectWithCompletedExperiments(t *testing.T) {
	scheme := newTestScheme()
	exp1 := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-1", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "5s",
				ReconcileCycles: 3,
			},
		},
	}
	exp2 := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-2", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Failed,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Failed,
				RecoveryTime:    "30s",
				ReconcileCycles: 10,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp1, exp2).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Failed"} 1
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Resilient"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_verdicts")
	assert.NoError(t, err)
}

func TestCollectRecoveryHistogram(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-hist", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "5s",
				ReconcileCycles: 3,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// 5s observation: lands in le="5" and all higher buckets, sum=5, count=1
	expected := `
# HELP chaosexperiment_recovery_seconds Recovery time distribution
# TYPE chaosexperiment_recovery_seconds histogram
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="0.5"} 0
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="1"} 0
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="2"} 0
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="5"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="10"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="30"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="60"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="120"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="300"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="+Inf"} 1
chaosexperiment_recovery_seconds_sum{component="predictor",injection_type="PodKill",operator="kserve"} 5
chaosexperiment_recovery_seconds_count{component="predictor",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_recovery_seconds")
	assert.NoError(t, err)
}

func TestCollectRecoveryHistogramInvalidDuration(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-bad", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:      v1alpha1.Resilient,
				RecoveryTime: "not-a-duration",
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Resilient"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_verdicts")
	assert.NoError(t, err)

	// Verify no recovery histogram was emitted for the invalid duration
	histCount := testutil.CollectAndCount(collector, "chaosexperiment_recovery_seconds")
	assert.Equal(t, 0, histCount, "invalid duration should not produce recovery histogram")
}

func TestCollectReconcileCyclesHistogram(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "exp-cycles", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "5s",
				ReconcileCycles: 3,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// 3 cycles observation: lands in le="3" and all higher buckets, sum=3, count=1
	expected := `
# HELP chaosexperiment_recovery_cycles Reconcile cycles needed per recovery
# TYPE chaosexperiment_recovery_cycles histogram
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="1"} 0
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="2"} 0
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="3"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="5"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="10"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="20"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="50"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="+Inf"} 1
chaosexperiment_recovery_cycles_sum{component="predictor",injection_type="PodKill",operator="kserve"} 3
chaosexperiment_recovery_cycles_count{component="predictor",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_recovery_cycles")
	assert.NoError(t, err)
}

func TestCollectWithActiveExperiments(t *testing.T) {
	scheme := newTestScheme()
	phases := []v1alpha1.ExperimentPhase{
		v1alpha1.PhaseSteadyStatePre,
		v1alpha1.PhaseInjecting,
		v1alpha1.PhaseObserving,
		v1alpha1.PhaseSteadyStatePost,
		v1alpha1.PhaseEvaluating,
	}

	var objs []client.Object
	for i, phase := range phases {
		objs = append(objs, &v1alpha1.ChaosExperiment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("active-%d", i),
				Namespace: "opendatahub",
			},
			Spec: v1alpha1.ChaosExperimentSpec{
				Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
				Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
			},
			Status: v1alpha1.ChaosExperimentStatus{Phase: phase},
		})
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_active_experiments Currently running experiments
# TYPE chaosexperiment_active_experiments gauge
chaosexperiment_active_experiments{operator="kserve"} 5
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_active_experiments")
	assert.NoError(t, err)
}

func TestCollectInjectionTotal(t *testing.T) {
	scheme := newTestScheme()
	completed := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
		},
	}
	active := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "active", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhaseInjecting},
	}
	aborted := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "aborted", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhaseAborted},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(completed, active, aborted).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_injections Computed injection counts from current CR set
# TYPE chaosexperiment_injections gauge
chaosexperiment_injections{component="predictor",injection_type="PodKill",operator="kserve"} 3
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_injections")
	assert.NoError(t, err)
}

func TestCollectPendingExperimentsIgnored(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhasePending},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	count := testutil.CollectAndCount(collector)
	assert.Equal(t, 0, count, "pending experiments should not produce metrics")
}

func TestCollectEmpty(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	collector := NewExperimentCollector(c)

	count := testutil.CollectAndCount(collector)
	assert.Equal(t, 0, count, "no experiments should produce no metrics")
}

func TestCollectWithNilFields(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "nil-eval", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:            v1alpha1.PhaseComplete,
			Verdict:          v1alpha1.Resilient,
			EvaluationResult: nil,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Resilient"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_verdicts")
	assert.NoError(t, err)
}

func TestCollectZeroReconcileCyclesExcluded(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "zero-cycles", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "5s",
				ReconcileCycles: 0, // zero value should not be observed
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	// recovery_cycles should not be emitted, recovery_seconds should be
	cyclesCount := testutil.CollectAndCount(collector, "chaosexperiment_recovery_cycles")
	assert.Equal(t, 0, cyclesCount, "zero ReconcileCycles should not produce histogram")
}

func TestCollectEmptyRecoveryTimeExcluded(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-recovery", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "", // empty string should not be observed
				ReconcileCycles: 3,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	// recovery_seconds should not be emitted, recovery_cycles should be
	secsCount := testutil.CollectAndCount(collector, "chaosexperiment_recovery_seconds")
	assert.Equal(t, 0, secsCount, "empty RecoveryTime should not produce histogram")

	cyclesCount := testutil.CollectAndCount(collector, "chaosexperiment_recovery_cycles")
	assert.Greater(t, cyclesCount, 0, "ReconcileCycles should still produce histogram")
}

func TestCollectAggregation(t *testing.T) {
	scheme := newTestScheme()
	var objs []client.Object
	for i := 0; i < 3; i++ {
		objs = append(objs, &v1alpha1.ChaosExperiment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("agg-%d", i),
				Namespace: "opendatahub",
			},
			Spec: v1alpha1.ChaosExperimentSpec{
				Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
				Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
			},
			Status: v1alpha1.ChaosExperimentStatus{
				Phase:   v1alpha1.PhaseComplete,
				Verdict: v1alpha1.Resilient,
			},
		})
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Resilient"} 3
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_verdicts")
	assert.NoError(t, err)
}

func TestCollectMultiObservationHistogram(t *testing.T) {
	scheme := newTestScheme()
	exp1 := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-1", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "2s",
				ReconcileCycles: 2,
			},
		},
	}
	exp2 := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-2", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Resilient,
				RecoveryTime:    "30s",
				ReconcileCycles: 10,
			},
		},
	}
	exp3 := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-3", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Failed,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Failed,
				RecoveryTime:    "120s",
				ReconcileCycles: 20,
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp1, exp2, exp3).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// 3 observations: 2s, 30s, 120s => sum=152, count=3
	// le=0.5: 0, le=1: 0, le=2: 1, le=5: 1, le=10: 1, le=30: 2, le=60: 2, le=120: 3, le=300: 3, +Inf: 3
	expectedSecs := `
# HELP chaosexperiment_recovery_seconds Recovery time distribution
# TYPE chaosexperiment_recovery_seconds histogram
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="0.5"} 0
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="1"} 0
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="2"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="5"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="10"} 1
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="30"} 2
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="60"} 2
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="120"} 3
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="300"} 3
chaosexperiment_recovery_seconds_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="+Inf"} 3
chaosexperiment_recovery_seconds_sum{component="predictor",injection_type="PodKill",operator="kserve"} 152
chaosexperiment_recovery_seconds_count{component="predictor",injection_type="PodKill",operator="kserve"} 3
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expectedSecs), "chaosexperiment_recovery_seconds")
	assert.NoError(t, err)

	// 3 observations: 2, 10, 20 => sum=32, count=3
	// le=1: 0, le=2: 1, le=3: 1, le=5: 1, le=10: 2, le=20: 3, le=50: 3, +Inf: 3
	expectedCycles := `
# HELP chaosexperiment_recovery_cycles Reconcile cycles needed per recovery
# TYPE chaosexperiment_recovery_cycles histogram
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="1"} 0
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="2"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="3"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="5"} 1
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="10"} 2
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="20"} 3
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="50"} 3
chaosexperiment_recovery_cycles_bucket{component="predictor",injection_type="PodKill",operator="kserve",le="+Inf"} 3
chaosexperiment_recovery_cycles_sum{component="predictor",injection_type="PodKill",operator="kserve"} 32
chaosexperiment_recovery_cycles_count{component="predictor",injection_type="PodKill",operator="kserve"} 3
`
	err = testutil.GatherAndCompare(reg, strings.NewReader(expectedCycles), "chaosexperiment_recovery_cycles")
	assert.NoError(t, err)
}

func TestCollectMultiLabelCombinations(t *testing.T) {
	scheme := newTestScheme()
	kserveExp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "kserve-exp", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
		},
	}
	dashboardExp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "dashboard-exp", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "dashboard", Component: "frontend"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.NetworkPartition},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Failed,
		},
	}
	activeExp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "modelmesh-active", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "modelmesh", Component: "controller"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.CRDMutation},
		},
		Status: v1alpha1.ChaosExperimentStatus{Phase: v1alpha1.PhaseInjecting},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(kserveExp, dashboardExp, activeExp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expectedVerdicts := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="frontend",injection_type="NetworkPartition",operator="dashboard",verdict="Failed"} 1
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Resilient"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expectedVerdicts), "chaosexperiment_verdicts")
	assert.NoError(t, err)

	expectedInjections := `
# HELP chaosexperiment_injections Computed injection counts from current CR set
# TYPE chaosexperiment_injections gauge
chaosexperiment_injections{component="controller",injection_type="CRDMutation",operator="modelmesh"} 1
chaosexperiment_injections{component="frontend",injection_type="NetworkPartition",operator="dashboard"} 1
chaosexperiment_injections{component="predictor",injection_type="PodKill",operator="kserve"} 1
`
	err = testutil.GatherAndCompare(reg, strings.NewReader(expectedInjections), "chaosexperiment_injections")
	assert.NoError(t, err)

	expectedActive := `
# HELP chaosexperiment_active_experiments Currently running experiments
# TYPE chaosexperiment_active_experiments gauge
chaosexperiment_active_experiments{operator="modelmesh"} 1
`
	err = testutil.GatherAndCompare(reg, strings.NewReader(expectedActive), "chaosexperiment_active_experiments")
	assert.NoError(t, err)
}

func TestCollectListError(t *testing.T) {
	scheme := newTestScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
				return fmt.Errorf("simulated list error")
			},
		}).
		Build()
	collector := NewExperimentCollector(c)

	count := testutil.CollectAndCount(collector)
	assert.Equal(t, 0, count, "list error should produce no metrics")
}

func TestCollectDeviations(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "dev-exp", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:    v1alpha1.Resilient,
				Deviations: []string{"slow_recovery: recovered in 45s, expected within 30s", "partial_reconciliation: not all resources reconciled"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_deviations Deviation counts by type from current CR set
# TYPE chaosexperiment_deviations gauge
chaosexperiment_deviations{component="predictor",deviation_type="partial_reconciliation",injection_type="PodKill",operator="kserve"} 1
chaosexperiment_deviations{component="predictor",deviation_type="slow_recovery",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_deviations")
	assert.NoError(t, err)
}

func TestCollectDeviationsDuplicateType(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "dup-dev", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Degraded,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:    v1alpha1.Degraded,
				Deviations: []string{"slow_recovery: A", "slow_recovery: B"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_deviations Deviation counts by type from current CR set
# TYPE chaosexperiment_deviations gauge
chaosexperiment_deviations{component="predictor",deviation_type="slow_recovery",injection_type="PodKill",operator="kserve"} 2
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_deviations")
	assert.NoError(t, err)
}

func TestCollectDeviationsWhitespace(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-dev", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Degraded,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:    v1alpha1.Degraded,
				Deviations: []string{" slow_recovery : recovered in 45s"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_deviations Deviation counts by type from current CR set
# TYPE chaosexperiment_deviations gauge
chaosexperiment_deviations{component="predictor",deviation_type="slow_recovery",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_deviations")
	assert.NoError(t, err)
}

func TestCollectDeviationsInvalidFormat(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "nocolon", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Degraded,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:    v1alpha1.Degraded,
				Deviations: []string{"no-colon-here"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	expected := `
# HELP chaosexperiment_deviations Deviation counts by type from current CR set
# TYPE chaosexperiment_deviations gauge
chaosexperiment_deviations{component="predictor",deviation_type="no-colon-here",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expected), "chaosexperiment_deviations")
	assert.NoError(t, err)
}

func TestCollectDeviationsEmpty(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "no-devs", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Resilient,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:    v1alpha1.Resilient,
				Deviations: []string{},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	devCount := testutil.CollectAndCount(collector, "chaosexperiment_deviations")
	assert.Equal(t, 0, devCount, "empty deviations should not produce metric")
}

func TestCollectDeviationsWithOtherMetrics(t *testing.T) {
	scheme := newTestScheme()
	exp := &v1alpha1.ChaosExperiment{
		ObjectMeta: metav1.ObjectMeta{Name: "full-exp", Namespace: "opendatahub"},
		Spec: v1alpha1.ChaosExperimentSpec{
			Target:    v1alpha1.TargetSpec{Operator: "kserve", Component: "predictor"},
			Injection: v1alpha1.InjectionSpec{Type: v1alpha1.PodKill},
		},
		Status: v1alpha1.ChaosExperimentStatus{
			Phase:   v1alpha1.PhaseComplete,
			Verdict: v1alpha1.Degraded,
			EvaluationResult: &v1alpha1.EvaluationSummary{
				Verdict:         v1alpha1.Degraded,
				RecoveryTime:    "45s",
				ReconcileCycles: 5,
				Deviations:      []string{"slow_recovery: recovered in 45s, expected within 30s"},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(exp).
		WithStatusSubresource(&v1alpha1.ChaosExperiment{}).
		Build()
	collector := NewExperimentCollector(c)

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	// Verify all metric families are emitted simultaneously
	for _, metricName := range []string{
		"chaosexperiment_verdicts",
		"chaosexperiment_injections",
		"chaosexperiment_deviations",
		"chaosexperiment_recovery_seconds",
		"chaosexperiment_recovery_cycles",
	} {
		count := testutil.CollectAndCount(collector, metricName)
		assert.Greater(t, count, 0, "expected %s to be emitted", metricName)
	}

	// Verify deviation value specifically
	expectedDev := `
# HELP chaosexperiment_deviations Deviation counts by type from current CR set
# TYPE chaosexperiment_deviations gauge
chaosexperiment_deviations{component="predictor",deviation_type="slow_recovery",injection_type="PodKill",operator="kserve"} 1
`
	err := testutil.GatherAndCompare(reg, strings.NewReader(expectedDev), "chaosexperiment_deviations")
	assert.NoError(t, err)

	// Verify verdict value specifically
	expectedVerdict := `
# HELP chaosexperiment_verdicts Computed verdict counts from current CR set
# TYPE chaosexperiment_verdicts gauge
chaosexperiment_verdicts{component="predictor",injection_type="PodKill",operator="kserve",verdict="Degraded"} 1
`
	err = testutil.GatherAndCompare(reg, strings.NewReader(expectedVerdict), "chaosexperiment_verdicts")
	assert.NoError(t, err)
}

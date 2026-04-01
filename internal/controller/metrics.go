package controller

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	descVerdicts = prometheus.NewDesc(
		"chaosexperiment_verdicts",
		"Computed verdict counts from current CR set",
		[]string{"operator", "component", "injection_type", "verdict"},
		nil,
	)
	descRecoverySeconds = prometheus.NewDesc(
		"chaosexperiment_recovery_seconds",
		"Recovery time distribution",
		[]string{"operator", "component", "injection_type"},
		nil,
	)
	descRecoveryCycles = prometheus.NewDesc(
		"chaosexperiment_recovery_cycles",
		"Reconcile cycles needed per recovery",
		[]string{"operator", "component", "injection_type"},
		nil,
	)
	descActiveExperiments = prometheus.NewDesc(
		"chaosexperiment_active_experiments",
		"Currently running experiments",
		[]string{"operator"},
		nil,
	)
	descInjections = prometheus.NewDesc(
		"chaosexperiment_injections",
		"Computed injection counts from current CR set",
		[]string{"operator", "component", "injection_type"},
		nil,
	)
	descDeviations = prometheus.NewDesc(
		"chaosexperiment_deviations",
		"Deviation counts by type from current CR set",
		[]string{"operator", "component", "injection_type", "deviation_type"},
		nil,
	)

	allDescs = []*prometheus.Desc{
		descVerdicts,
		descRecoverySeconds,
		descRecoveryCycles,
		descActiveExperiments,
		descInjections,
		descDeviations,
	}

	recoverySecondsBuckets = []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300}
	recoveryCyclesBuckets  = []float64{1, 2, 3, 5, 10, 20, 50}

	activePhases = map[v1alpha1.ExperimentPhase]bool{
		v1alpha1.PhaseSteadyStatePre:  true,
		v1alpha1.PhaseInjecting:       true,
		v1alpha1.PhaseObserving:       true,
		v1alpha1.PhaseSteadyStatePost: true,
		v1alpha1.PhaseEvaluating:      true,
	}
)

// ExperimentCollector implements prometheus.Collector by listing ChaosExperiment
// CRs from the controller-runtime cache on each scrape and computing metrics.
type ExperimentCollector struct {
	client client.Reader
}

// NewExperimentCollector creates a collector that reads CRs via the given client.
func NewExperimentCollector(client client.Reader) *ExperimentCollector {
	return &ExperimentCollector{client: client}
}

// Describe sends all metric descriptors unconditionally.
func (c *ExperimentCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range allDescs {
		ch <- d
	}
}

// Collect recomputes metrics from ChaosExperiment CRs on each scrape.
func (c *ExperimentCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var list v1alpha1.ChaosExperimentList
	if err := c.client.List(ctx, &list); err != nil {
		slog.Error("failed to list ChaosExperiments for metrics", "error", err)
		return
	}

	verdicts := map[verdictKey]float64{}
	injections := map[injectionKey]float64{}
	activeByOperator := map[string]float64{}
	deviations := map[deviationKey]float64{}
	type histObs struct {
		values []float64
	}
	recoverySecsObs := map[injectionKey]*histObs{}
	recoveryCyclesObs := map[injectionKey]*histObs{}

	for i := range list.Items {
		exp := &list.Items[i]
		operator := exp.Spec.Target.Operator
		component := exp.Spec.Target.Component
		injType := string(exp.Spec.Injection.Type)
		iKey := injectionKey{operator, component, injType}

		switch {
		case exp.Status.Phase == v1alpha1.PhaseComplete:
			verdict := string(exp.Status.Verdict)
			vKey := verdictKey{operator, component, injType, verdict}
			verdicts[vKey]++
			injections[iKey]++

			if exp.Status.EvaluationResult != nil {
				if exp.Status.EvaluationResult.RecoveryTime != "" {
					d, err := time.ParseDuration(exp.Status.EvaluationResult.RecoveryTime)
					if err == nil {
						if recoverySecsObs[iKey] == nil {
							recoverySecsObs[iKey] = &histObs{}
						}
						recoverySecsObs[iKey].values = append(recoverySecsObs[iKey].values, d.Seconds())
					}
				}
				if exp.Status.EvaluationResult.ReconcileCycles > 0 {
					if recoveryCyclesObs[iKey] == nil {
						recoveryCyclesObs[iKey] = &histObs{}
					}
					recoveryCyclesObs[iKey].values = append(recoveryCyclesObs[iKey].values, float64(exp.Status.EvaluationResult.ReconcileCycles))
				}
				for _, d := range exp.Status.EvaluationResult.Deviations {
					devType := strings.TrimSpace(strings.SplitN(d, ":", 2)[0])
					dKey := deviationKey{operator, component, injType, devType}
					deviations[dKey]++
				}
			}

		case activePhases[exp.Status.Phase]:
			activeByOperator[operator]++
			injections[iKey]++

		case exp.Status.Phase == v1alpha1.PhaseAborted:
			injections[iKey]++
		}
	}

	for k, v := range verdicts {
		ch <- prometheus.MustNewConstMetric(descVerdicts, prometheus.GaugeValue, v,
			k.operator, k.component, k.injectionType, k.verdict)
	}

	for k, v := range injections {
		ch <- prometheus.MustNewConstMetric(descInjections, prometheus.GaugeValue, v,
			k.operator, k.component, k.injectionType)
	}

	for op, v := range activeByOperator {
		ch <- prometheus.MustNewConstMetric(descActiveExperiments, prometheus.GaugeValue, v, op)
	}

	for k, v := range deviations {
		ch <- prometheus.MustNewConstMetric(descDeviations, prometheus.GaugeValue, v,
			k.operator, k.component, k.injectionType, k.deviationType)
	}

	for k, obs := range recoverySecsObs {
		bucketCounts, sum := computeHistogram(obs.values, recoverySecondsBuckets)
		ch <- prometheus.MustNewConstHistogram(descRecoverySeconds,
			uint64(len(obs.values)), sum, bucketCounts,
			k.operator, k.component, k.injectionType)
	}

	for k, obs := range recoveryCyclesObs {
		bucketCounts, sum := computeHistogram(obs.values, recoveryCyclesBuckets)
		ch <- prometheus.MustNewConstHistogram(descRecoveryCycles,
			uint64(len(obs.values)), sum, bucketCounts,
			k.operator, k.component, k.injectionType)
	}
}

type verdictKey struct {
	operator, component, injectionType, verdict string
}

type injectionKey struct {
	operator, component, injectionType string
}

type deviationKey struct {
	operator, component, injectionType, deviationType string
}

func computeHistogram(values []float64, bucketBounds []float64) (map[float64]uint64, float64) {
	counts := make(map[float64]uint64, len(bucketBounds))
	for _, b := range bucketBounds {
		counts[b] = 0
	}
	var sum float64
	for _, v := range values {
		sum += v
		for _, b := range bucketBounds {
			if v <= b {
				counts[b]++
			}
		}
	}
	return counts, sum
}

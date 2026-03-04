package injection

import (
	"context"
	"fmt"
	"math/rand/v2"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodKillInjector injects faults by force-deleting pods that match a label selector.
type PodKillInjector struct {
	client client.Client
}

// NewPodKillInjector creates a new PodKillInjector using the given Kubernetes client.
func NewPodKillInjector(c client.Client) *PodKillInjector {
	return &PodKillInjector{client: c}
}

func (p *PodKillInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validatePodKillParams(spec, blast)
}

// Inject force-deletes pods matching the label selector and returns a no-op cleanup function.
func (p *PodKillInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	selector, err := labels.Parse(spec.Parameters["labelSelector"])
	if err != nil {
		return nil, nil, fmt.Errorf("parsing label selector: %w", err)
	}

	podList := &corev1.PodList{}
	if err := p.client.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return nil, nil, fmt.Errorf("listing pods: %w", err)
	}

	if len(podList.Items) == 0 {
		return nil, nil, fmt.Errorf("no pods found matching selector %s in namespace %s", selector.String(), namespace)
	}

	killCount := spec.Count
	if killCount <= 0 {
		killCount = 1
	}
	if killCount > len(podList.Items) {
		killCount = len(podList.Items)
	}

	// Shuffle pods so selection is random rather than deterministic.
	rand.Shuffle(len(podList.Items), func(i, j int) {
		podList.Items[i], podList.Items[j] = podList.Items[j], podList.Items[i]
	})

	var events []v1alpha1.InjectionEvent
	gracePeriod := int64(0)

	for i := 0; i < killCount; i++ {
		pod := podList.Items[i]
		if err := p.client.Delete(ctx, &pod, &client.DeleteOptions{
			GracePeriodSeconds: &gracePeriod,
			Preconditions:      &metav1.Preconditions{UID: &pod.UID},
		}); err != nil {
			return nil, events, fmt.Errorf("killing pod %s: %w", pod.Name, err)
		}

		events = append(events, NewEvent(
			v1alpha1.PodKill,
			pod.Name,
			"deleted",
			map[string]string{
				"namespace": namespace,
				"node":      pod.Spec.NodeName,
			},
		))
	}

	// No cleanup needed -- Deployment controller will recreate pods
	cleanup := func(ctx context.Context) error { return nil }

	return cleanup, events, nil
}

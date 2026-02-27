package injection

import (
	"context"
	"fmt"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPartitionInjector struct {
	client client.Client
}

func NewNetworkPartitionInjector(c client.Client) *NetworkPartitionInjector {
	return &NetworkPartitionInjector{client: c}
}

func (n *NetworkPartitionInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	if _, ok := spec.Parameters["labelSelector"]; !ok {
		return fmt.Errorf("NetworkPartition requires 'labelSelector' parameter")
	}
	return nil
}

func (n *NetworkPartitionInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	selector, err := labels.Parse(spec.Parameters["labelSelector"])
	if err != nil {
		return nil, nil, fmt.Errorf("parsing label selector: %w", err)
	}

	policyName := fmt.Sprintf("odh-chaos-network-partition-%s", spec.Parameters["labelSelector"])
	// Truncate to K8s name limit
	if len(policyName) > 63 {
		policyName = policyName[:63]
	}

	matchLabels := map[string]string{}
	reqs, selectable := selector.Requirements()
	if selectable {
		for _, req := range reqs {
			op := req.Operator()
			if op == selection.Equals || op == selection.DoubleEquals || op == selection.In {
				vals := req.Values().List()
				if len(vals) == 1 {
					matchLabels[req.Key()] = vals[0]
				}
			}
		}
	}

	annotations := map[string]string{}
	if spec.TTL.Duration > 0 {
		annotations[safety.TTLAnnotationKey] = safety.TTLExpiry(spec.TTL.Duration)
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "odh-chaos",
				"chaos.opendatahub.io/type":    "network-partition",
			},
			Annotations: annotations,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// Empty ingress/egress = deny all
		},
	}

	if err := n.client.Create(ctx, policy); err != nil {
		return nil, nil, fmt.Errorf("creating NetworkPolicy: %w", err)
	}

	events := []v1alpha1.InjectionEvent{
		NewEvent(v1alpha1.NetworkPartition, policyName, "created",
			map[string]string{
				"namespace": namespace,
				"selector":  spec.Parameters["labelSelector"],
			}),
	}

	cleanup := func(ctx context.Context) error {
		return n.client.Delete(ctx, policy)
	}

	return cleanup, events, nil
}

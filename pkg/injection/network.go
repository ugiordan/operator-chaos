package injection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// invalidK8sNameChars matches characters that are not valid in Kubernetes resource names.
var invalidK8sNameChars = regexp.MustCompile(`[^a-z0-9\-]`)

// NetworkPartitionInjector injects faults by creating a deny-all NetworkPolicy for pods matching a label selector.
type NetworkPartitionInjector struct {
	client client.Client
}

// NewNetworkPartitionInjector creates a new NetworkPartitionInjector using the given Kubernetes client.
func NewNetworkPartitionInjector(c client.Client) *NetworkPartitionInjector {
	return &NetworkPartitionInjector{client: c}
}

func (n *NetworkPartitionInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
	return validateNetworkPartitionParams(spec)
}

// Inject creates a deny-all NetworkPolicy targeting the selected pods and returns a cleanup function that deletes it.
func (n *NetworkPartitionInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
	selector, err := labels.Parse(spec.Parameters["labelSelector"])
	if err != nil {
		return nil, nil, fmt.Errorf("parsing label selector: %w", err)
	}

	policyName := sanitizeK8sName("odh-chaos-np-", spec.Parameters["labelSelector"])

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
			Name:        policyName,
			Namespace:   namespace,
			Labels:      safety.ChaosLabels(string(v1alpha1.NetworkPartition)),
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

// sanitizeK8sName creates a valid Kubernetes resource name from a prefix and input string.
// It replaces invalid characters with hyphens, truncates if necessary, and appends a hash
// suffix for uniqueness when truncation is needed.
func sanitizeK8sName(prefix, input string) string {
	sanitized := invalidK8sNameChars.ReplaceAllString(strings.ToLower(input), "-")

	name := prefix + sanitized
	if len(name) > 63 {
		// Truncate and add hash suffix for uniqueness
		hash := sha256.Sum256([]byte(input))
		suffix := hex.EncodeToString(hash[:4])
		name = name[:54] + "-" + suffix
	}
	// Ensure name ends with alphanumeric
	name = strings.TrimRight(name, "-.")
	if len(name) == 0 {
		hash := sha256.Sum256([]byte(input))
		name = prefix + hex.EncodeToString(hash[:8])
	}
	return name
}

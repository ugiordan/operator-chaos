package olm

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WatchUpgrade monitors the full OLM chain (Subscription → InstallPlan → CSV)
// and sends status updates on the returned channel. The channel is closed when
// the upgrade reaches Succeeded, Failed, or the context is cancelled.
// pollInterval controls how frequently OLM objects are polled.
func (c *Client) WatchUpgrade(ctx context.Context, operator, namespace string, pollInterval time.Duration) (<-chan UpgradeStatus, error) {
	// Read initial subscription state
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group: subscriptionGVR.Group, Version: subscriptionGVR.Version, Kind: "Subscription",
	})
	if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, sub); err != nil {
		return nil, fmt.Errorf("getting Subscription %s/%s: %w", namespace, operator, err)
	}

	initialInstalled, _, err := getSubscriptionStatus(sub)
	if err != nil {
		return nil, fmt.Errorf("reading initial subscription status: %w", err)
	}

	ch := make(chan UpgradeStatus, 16)

	go func() {
		defer close(ch)
		c.watchLoop(ctx, operator, namespace, initialInstalled, pollInterval, ch)
	}()

	return ch, nil
}

func (c *Client) watchLoop(ctx context.Context, operator, namespace, initialInstalledCSV string, pollInterval time.Duration, ch chan<- UpgradeStatus) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var (
		seenInstallPlan bool
		installPlanName string
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Step 1: Check subscription for new currentCSV
		sub := &unstructured.Unstructured{}
		sub.SetGroupVersionKind(schema.GroupVersionKind{
			Group: subscriptionGVR.Group, Version: subscriptionGVR.Version, Kind: "Subscription",
		})
		if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, sub); err != nil {
			c.logger.Printf("watch: error getting subscription: %v", err)
			continue
		}

		installed, current, err := getSubscriptionStatus(sub)
		if err != nil {
			c.logger.Printf("watch: error reading subscription status: %v", err)
			continue
		}
		channel, err := getSubscriptionChannel(sub)
		if err != nil {
			c.logger.Printf("watch: error reading subscription channel: %v", err)
			continue
		}

		// If current hasn't changed from initial, upgrade hasn't started
		if current == initialInstalledCSV || current == "" {
			ch <- UpgradeStatus{
				Phase:        PhasePending,
				Subscription: operator,
				Channel:      channel,
				InstalledCSV: installed,
				Message:      "waiting for new CSV version",
			}
			continue
		}

		// Step 2: Find InstallPlan
		if !seenInstallPlan {
			ipName, ipNS, _ := getSubscriptionInstallPlanRef(sub)
			if ipName == "" {
				ch <- UpgradeStatus{
					Phase:        PhasePending,
					Subscription: operator,
					Channel:      channel,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      "waiting for InstallPlan",
				}
				continue
			}
			installPlanName = ipName
			if ipNS == "" {
				ipNS = namespace
			}

			ch <- UpgradeStatus{
				Phase:        PhaseInstallPlanCreated,
				Subscription: operator,
				Channel:      channel,
				InstallPlan:  ipName,
				CurrentCSV:   current,
				InstalledCSV: installed,
				Message:      fmt.Sprintf("InstallPlan created: %s", ipName),
			}

			// Check approval
			ip := &unstructured.Unstructured{}
			ip.SetGroupVersionKind(schema.GroupVersionKind{
				Group: installPlanGVR.Group, Version: installPlanGVR.Version, Kind: "InstallPlan",
			})
			if err := c.k8s.Get(ctx, client.ObjectKey{Name: ipName, Namespace: ipNS}, ip); err != nil {
				c.logger.Printf("watch: error getting InstallPlan: %v", err)
				continue
			}

			approval, _ := getInstallPlanApproval(ip)
			ipPhase, _ := getInstallPlanPhase(ip)

			if approval == "Manual" && ipPhase != "Complete" {
				ch <- UpgradeStatus{
					Phase:               PhaseInstallPlanCreated,
					Subscription:        operator,
					Channel:             channel,
					InstallPlan:         ipName,
					InstallPlanApproval: "Manual",
					CurrentCSV:          current,
					InstalledCSV:        installed,
					Message:             "InstallPlan requires manual approval",
				}
				continue
			}

			switch ipPhase {
			case "Complete":
				seenInstallPlan = true
				ch <- UpgradeStatus{
					Phase:               PhaseInstallPlanApproved,
					Subscription:        operator,
					Channel:             channel,
					InstallPlan:         ipName,
					InstallPlanApproval: approval,
					CurrentCSV:          current,
					InstalledCSV:        installed,
					Message:             "InstallPlan complete",
				}
			case "Failed":
				ch <- UpgradeStatus{
					Phase:        PhaseFailed,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  ipName,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      fmt.Sprintf("InstallPlan %s failed", ipName),
				}
				return
			default:
				// InstallPlan still in progress, keep polling
				ch <- UpgradeStatus{
					Phase:        PhaseInstallPlanCreated,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  ipName,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      fmt.Sprintf("InstallPlan %s phase: %s", ipName, ipPhase),
				}
				continue
			}
		}

		// Step 3: Watch CSV
		if current != "" {
			csv := &unstructured.Unstructured{}
			csv.SetGroupVersionKind(schema.GroupVersionKind{
				Group: csvGVR.Group, Version: csvGVR.Version, Kind: "ClusterServiceVersion",
			})
			if err := c.k8s.Get(ctx, client.ObjectKey{Name: current, Namespace: namespace}, csv); err != nil {
				ch <- UpgradeStatus{
					Phase:        PhaseInstalling,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  installPlanName,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      fmt.Sprintf("waiting for CSV %s", current),
				}
				continue
			}

			csvPhase, _ := getCSVPhase(csv)

			switch csvPhase {
			case "Succeeded":
				// Verify subscription's installedCSV has been updated to confirm OLM rollout is complete
				finalInstalled := installed
				if installed != current {
					// Re-read subscription to check if OLM has updated installedCSV
					freshSub := &unstructured.Unstructured{}
					freshSub.SetGroupVersionKind(schema.GroupVersionKind{
						Group: subscriptionGVR.Group, Version: subscriptionGVR.Version, Kind: "Subscription",
					})
					if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, freshSub); err == nil {
						fi, _, _ := getSubscriptionStatus(freshSub)
						finalInstalled = fi
					}
				}
				ch <- UpgradeStatus{
					Phase:        PhaseSucceeded,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  installPlanName,
					CurrentCSV:   current,
					InstalledCSV: finalInstalled,
					Message:      fmt.Sprintf("CSV %s Succeeded", current),
				}
				return
			case "Failed":
				ch <- UpgradeStatus{
					Phase:        PhaseFailed,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  installPlanName,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      fmt.Sprintf("CSV %s Failed", current),
				}
				return
			default:
				ch <- UpgradeStatus{
					Phase:        PhaseInstalling,
					Subscription: operator,
					Channel:      channel,
					InstallPlan:  installPlanName,
					CurrentCSV:   current,
					InstalledCSV: installed,
					Message:      fmt.Sprintf("CSV %s phase: %s", current, csvPhase),
				}
			}
		}
	}
}

package olm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client provides OLM operations for upgrade workflows.
type Client struct {
	k8s    client.Client
	logger *log.Logger
}

// NewClient creates a new OLM client.
func NewClient(k8s client.Client, logger *log.Logger) *Client {
	return &Client{k8s: k8s, logger: logger}
}

// Discover lists available channels for an operator from the PackageManifest.
func (c *Client) Discover(ctx context.Context, operator, namespace string) ([]ChannelInfo, error) {
	pm := &unstructured.Unstructured{}
	pm.SetGroupVersionKind(schema.GroupVersionKind{
		Group: packageManifestGVR.Group, Version: packageManifestGVR.Version, Kind: "PackageManifest",
	})

	if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, pm); err != nil {
		return nil, fmt.Errorf("getting PackageManifest %s/%s: %w", namespace, operator, err)
	}

	channels, err := getPackageChannels(pm)
	if err != nil {
		return nil, fmt.Errorf("extracting channels: %w", err)
	}

	return channels, nil
}

// PatchChannel updates the Subscription's spec.channel to trigger an upgrade.
func (c *Client) PatchChannel(ctx context.Context, operator, namespace, channel string) error {
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group: subscriptionGVR.Group, Version: subscriptionGVR.Version, Kind: "Subscription",
	})

	if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, sub); err != nil {
		return fmt.Errorf("getting Subscription %s/%s: %w", namespace, operator, err)
	}

	patchObj := map[string]interface{}{
		"spec": map[string]interface{}{
			"channel": channel,
		},
	}
	patch, err := json.Marshal(patchObj)
	if err != nil {
		return fmt.Errorf("marshaling channel patch: %w", err)
	}
	if err := c.k8s.Patch(ctx, sub, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("patching Subscription channel to %s: %w", channel, err)
	}

	c.logger.Printf("patched Subscription %s/%s channel to %s", namespace, operator, channel)
	return nil
}

// GetCurrentVersion returns the installed CSV version for an operator.
func (c *Client) GetCurrentVersion(ctx context.Context, operator, namespace string) (string, error) {
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group: subscriptionGVR.Group, Version: subscriptionGVR.Version, Kind: "Subscription",
	})

	if err := c.k8s.Get(ctx, client.ObjectKey{Name: operator, Namespace: namespace}, sub); err != nil {
		return "", fmt.Errorf("getting Subscription %s/%s: %w", namespace, operator, err)
	}

	installed, _, err := getSubscriptionStatus(sub)
	if err != nil {
		return "", fmt.Errorf("reading subscription status: %w", err)
	}

	return installed, nil
}

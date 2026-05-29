package chaosclient

import (
	"context"
	"sync/atomic"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sdk "github.com/opendatahub-io/operator-chaos/pkg/sdk"
)

// ChaosClient wraps a controller-runtime client.Client with fault injection.
// It embeds the inner client so any interface methods not explicitly overridden
// (e.g., Apply in newer controller-runtime versions) are automatically delegated.
// CRUD operations check the FaultConfig before delegating to the inner client.
// The fault config is stored as an atomic pointer for safe concurrent updates.
type ChaosClient struct {
	client.Client
	faults atomic.Pointer[sdk.FaultConfig]
}

// NewChaosClient creates a new ChaosClient wrapping the given inner client.
// If faults is nil, all operations pass through without fault injection.
func NewChaosClient(inner client.Client, faults *sdk.FaultConfig) *ChaosClient {
	c := &ChaosClient{Client: inner}
	if faults != nil {
		c.faults.Store(faults)
	}
	return c
}

func (c *ChaosClient) getFaults() *sdk.FaultConfig {
	return c.faults.Load()
}

// Get retrieves an object, with optional fault injection.
func (c *ChaosClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpGet); err != nil {
		return err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

// List retrieves a list of objects, with optional fault injection.
func (c *ChaosClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpList); err != nil {
		return err
	}
	return c.Client.List(ctx, list, opts...)
}

// Create saves a new object, with optional fault injection.
func (c *ChaosClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpCreate); err != nil {
		return err
	}
	return c.Client.Create(ctx, obj, opts...)
}

// Delete removes an object, with optional fault injection.
func (c *ChaosClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpDelete); err != nil {
		return err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

// Update modifies an existing object, with optional fault injection.
func (c *ChaosClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpUpdate); err != nil {
		return err
	}
	return c.Client.Update(ctx, obj, opts...)
}

// Patch applies a patch to an object, with optional fault injection.
func (c *ChaosClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpPatch); err != nil {
		return err
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the options, with optional fault injection.
func (c *ChaosClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if err := c.getFaults().MaybeInject(sdk.OpDeleteAllOf); err != nil {
		return err
	}
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

// UpdateFaultConfig replaces the current fault configuration atomically.
func (c *ChaosClient) UpdateFaultConfig(fc *sdk.FaultConfig) {
	c.faults.Store(fc)
}

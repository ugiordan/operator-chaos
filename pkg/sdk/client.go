package sdk

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time assertion: ChaosClient implements client.Client.
var _ client.Client = (*ChaosClient)(nil)

// ChaosClient wraps a controller-runtime client.Client with fault injection.
// CRUD operations check the FaultConfig before delegating to the inner client.
// Metadata methods (Scheme, RESTMapper, etc.) always delegate directly.
type ChaosClient struct {
	inner  client.Client
	faults *FaultConfig
}

// NewChaosClient creates a new ChaosClient wrapping the given inner client.
// If faults is nil, all operations pass through without fault injection.
func NewChaosClient(inner client.Client, faults *FaultConfig) *ChaosClient {
	return &ChaosClient{
		inner:  inner,
		faults: faults,
	}
}

// Get retrieves an object, with optional fault injection.
func (c *ChaosClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.faults.MaybeInject(string(OpGet)); err != nil {
		return err
	}
	return c.inner.Get(ctx, key, obj, opts...)
}

// List retrieves a list of objects, with optional fault injection.
func (c *ChaosClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := c.faults.MaybeInject(string(OpList)); err != nil {
		return err
	}
	return c.inner.List(ctx, list, opts...)
}

// Apply applies the given apply configuration, with optional fault injection.
func (c *ChaosClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	if err := c.faults.MaybeInject("apply"); err != nil {
		return err
	}
	return c.inner.Apply(ctx, obj, opts...)
}

// Create saves a new object, with optional fault injection.
func (c *ChaosClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.faults.MaybeInject(string(OpCreate)); err != nil {
		return err
	}
	return c.inner.Create(ctx, obj, opts...)
}

// Delete removes an object, with optional fault injection.
func (c *ChaosClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := c.faults.MaybeInject(string(OpDelete)); err != nil {
		return err
	}
	return c.inner.Delete(ctx, obj, opts...)
}

// Update modifies an existing object, with optional fault injection.
func (c *ChaosClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if err := c.faults.MaybeInject(string(OpUpdate)); err != nil {
		return err
	}
	return c.inner.Update(ctx, obj, opts...)
}

// Patch applies a patch to an object, with optional fault injection.
func (c *ChaosClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.faults.MaybeInject(string(OpPatch)); err != nil {
		return err
	}
	return c.inner.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the options, with optional fault injection.
func (c *ChaosClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if err := c.faults.MaybeInject(string(OpDeleteAllOf)); err != nil {
		return err
	}
	return c.inner.DeleteAllOf(ctx, obj, opts...)
}

// Status returns a SubResourceWriter for the status subresource.
// Delegates directly to the inner client.
func (c *ChaosClient) Status() client.SubResourceWriter {
	return c.inner.Status()
}

// SubResource returns a SubResourceClient for the named subresource.
// Delegates directly to the inner client.
func (c *ChaosClient) SubResource(subResource string) client.SubResourceClient {
	return c.inner.SubResource(subResource)
}

// Scheme returns the scheme used by the inner client.
func (c *ChaosClient) Scheme() *runtime.Scheme {
	return c.inner.Scheme()
}

// RESTMapper returns the REST mapper used by the inner client.
func (c *ChaosClient) RESTMapper() meta.RESTMapper {
	return c.inner.RESTMapper()
}

// GroupVersionKindFor returns the GroupVersionKind for the given object.
func (c *ChaosClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.inner.GroupVersionKindFor(obj)
}

// IsObjectNamespaced returns true if the object's GroupVersionKind is namespaced.
func (c *ChaosClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.inner.IsObjectNamespaced(obj)
}

package fuzz

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Invariant is a function that checks a post-reconcile condition.
// It receives the client (after reconciliation) and returns an error if the invariant is violated.
type Invariant func(ctx context.Context, c client.Client) error

// ObjectExists returns an invariant that checks a specific object still exists after reconciliation.
func ObjectExists(key client.ObjectKey, obj client.Object) Invariant {
	return func(ctx context.Context, c client.Client) error {
		if err := c.Get(ctx, key, obj); err != nil {
			return fmt.Errorf("expected object %s to exist: %w", key, err)
		}
		return nil
	}
}

// ObjectCount returns an invariant that checks the count of objects of a given type.
func ObjectCount(list client.ObjectList, expected int, opts ...client.ListOption) Invariant {
	return func(ctx context.Context, c client.Client) error {
		if err := c.List(ctx, list, opts...); err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}

		items, err := meta.ExtractList(list)
		if err != nil {
			return fmt.Errorf("failed to extract items from list: %w", err)
		}
		if len(items) != expected {
			return fmt.Errorf("expected %d objects, got %d", expected, len(items))
		}
		return nil
	}
}

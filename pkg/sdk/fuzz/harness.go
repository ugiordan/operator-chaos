package fuzz

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// defaultRunTimeout is the maximum time a single fuzz iteration is allowed to run.
const defaultRunTimeout = 5 * time.Second

// ReconcilerFactory creates a fresh reconciler wired to the given client.
// This is the primary integration point for operator teams: implement a factory
// that constructs your reconciler with the provided client.
type ReconcilerFactory func(c client.Client) reconcile.Reconciler

// Harness provides a reusable fuzz testing harness for operator reconcilers.
type Harness struct {
	factory    ReconcilerFactory
	scheme     *runtime.Scheme
	objects    []client.Object
	invariants []Invariant
	request    reconcile.Request
	timeout    time.Duration
}

// NewHarness creates a fuzz harness for the given reconciler factory.
func NewHarness(factory ReconcilerFactory, scheme *runtime.Scheme, req reconcile.Request, seedObjects ...client.Object) *Harness {
	return &Harness{
		factory:    factory,
		scheme:     scheme,
		objects:    seedObjects,
		request:    req,
		timeout:    defaultRunTimeout,
		invariants: nil,
	}
}

// AddInvariant adds a post-reconcile invariant check.
func (h *Harness) AddInvariant(inv Invariant) {
	h.invariants = append(h.invariants, inv)
}

// SetTimeout overrides the default per-iteration timeout.
func (h *Harness) SetTimeout(d time.Duration) {
	h.timeout = d
}

// Run executes one fuzz iteration: creates a fake client, wraps it with ChaosClient
// using the given FaultConfig, constructs a reconciler via the factory, reconciles,
// then checks all invariants.
//
// Returns an error if:
//   - the reconciler panics (a real bug)
//   - the reconciler returns a non-chaos error (a real bug)
//   - any invariant is violated (a real bug)
//
// Chaos-injected errors (sdk.ChaosError) are expected and silently ignored.
func (h *Harness) Run(t *testing.T, fc *sdk.FaultConfig) error {
	t.Helper()

	// Build a fresh fake client with seed objects.
	builder := fake.NewClientBuilder().WithScheme(h.scheme)
	if len(h.objects) > 0 {
		builder = builder.WithObjects(h.objects...)
	}
	fakeClient := builder.Build()

	// Wrap with ChaosClient.
	chaosClient := sdk.NewChaosClient(fakeClient, fc)

	// Create a fresh reconciler with the chaos client injected.
	rec := h.factory(chaosClient)

	// Create a context with timeout to prevent fuzz workers from hanging.
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	// Run reconciliation, catching panics.
	var reconcileErr error
	var panicked bool
	var stackTrace []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				stackTrace = debug.Stack()
				reconcileErr = fmt.Errorf("panic during reconciliation: %v\n%s", r, stackTrace)
				panicked = true
			}
		}()
		_, reconcileErr = rec.Reconcile(ctx, h.request)
	}()

	// If the reconciler panicked, that is a real bug — return immediately.
	if panicked {
		return reconcileErr
	}

	// Distinguish chaos-injected errors (expected) from real errors (bugs).
	if reconcileErr != nil {
		var chaosErr *sdk.ChaosError
		if !errors.As(reconcileErr, &chaosErr) {
			return fmt.Errorf("non-chaos reconcile error: %w", reconcileErr)
		}
		// Chaos error — expected, continue to invariant checks.
	}

	// Check invariants using the underlying fake client (not the chaos client).
	for i, inv := range h.invariants {
		if err := inv(ctx, fakeClient); err != nil {
			return fmt.Errorf("invariant %d violated: %w", i, err)
		}
	}

	return nil
}

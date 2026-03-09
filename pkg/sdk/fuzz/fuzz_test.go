package fuzz

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// --- mock reconcilers ---

// mockReconciler is a test reconciler that does a simple Get.
type mockReconciler struct {
	client client.Client
}

func (m *mockReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	cm := &corev1.ConfigMap{}
	if err := m.client.Get(ctx, req.NamespacedName, cm); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func mockFactory(c client.Client) reconcile.Reconciler {
	return &mockReconciler{client: c}
}

// panicReconciler panics during reconciliation.
type panicReconciler struct{}

func (p *panicReconciler) Reconcile(_ context.Context, _ reconcile.Request) (ctrl.Result, error) {
	panic("unexpected state")
}

func panicFactory(c client.Client) reconcile.Reconciler {
	return &panicReconciler{}
}

// realErrorReconciler returns a non-chaos error.
type realErrorReconciler struct{}

func (r *realErrorReconciler) Reconcile(_ context.Context, _ reconcile.Request) (ctrl.Result, error) {
	return ctrl.Result{}, fmt.Errorf("real bug: nil pointer in handler")
}

func realErrorFactory(c client.Client) reconcile.Reconciler {
	return &realErrorReconciler{}
}

// --- helpers ---

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func testConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}
}

func testRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-cm",
			Namespace: "default",
		},
	}
}

// --- DecodeFaultConfig tests ---

func TestDecodeFaultConfig_ZeroMask(t *testing.T) {
	fc := DecodeFaultConfig(0, 0, 32768)
	assert.True(t, fc.IsActive())
	for _, op := range allOperations {
		err := fc.MaybeInject(op)
		assert.NoError(t, err, "expected no fault for op %s with zero mask", op)
	}
}

func TestDecodeFaultConfig_AllOps(t *testing.T) {
	fc := DecodeFaultConfig(0x01FF, 0, 65535)
	assert.True(t, fc.IsActive())
	for _, op := range allOperations {
		err := fc.MaybeInject(op)
		assert.Error(t, err, "expected fault for op %s with full mask and max intensity", op)
	}
}

func TestDecodeFaultConfig_SingleOp(t *testing.T) {
	fc := DecodeFaultConfig(1<<2, 1, 65535)
	assert.True(t, fc.IsActive())
	err := fc.MaybeInject(sdk.OpCreate)
	assert.Error(t, err)
	err = fc.MaybeInject(sdk.OpGet)
	assert.NoError(t, err)
}

func TestDecodeFaultConfig_ZeroIntensity(t *testing.T) {
	fc := DecodeFaultConfig(0x01FF, 0, 0)
	for _, op := range allOperations {
		err := fc.MaybeInject(op)
		assert.NoError(t, err, "expected no fault for op %s with zero intensity", op)
	}
}

func TestDecodeFaultConfig_FaultTypeWraps(t *testing.T) {
	fc := DecodeFaultConfig(1, 255, 65535)
	err := fc.MaybeInject(sdk.OpGet)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chaos(get):")
}

// --- Harness tests ---

func TestHarness_RunSuccess(t *testing.T) {
	h := NewHarness(mockFactory, testScheme(), testRequest(), testConfigMap())
	fc := sdk.NewFaultConfig(nil)
	err := h.Run(t, fc)
	assert.NoError(t, err)
}

func TestHarness_RunWithChaosFaults(t *testing.T) {
	h := NewHarness(mockFactory, testScheme(), testRequest(), testConfigMap())
	// Fault on Get with 100% rate — chaos error, should be silently ignored.
	fc := DecodeFaultConfig(1, 0, 65535)
	err := h.Run(t, fc)
	assert.NoError(t, err, "chaos-injected reconcile errors should not be treated as failures")
}

func TestHarness_RunDetectsRealErrors(t *testing.T) {
	h := NewHarness(realErrorFactory, testScheme(), testRequest())
	fc := sdk.NewFaultConfig(nil)
	err := h.Run(t, fc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-chaos reconcile error")
	assert.Contains(t, err.Error(), "real bug")
}

func TestHarness_RunPanic(t *testing.T) {
	h := NewHarness(panicFactory, testScheme(), testRequest())
	fc := sdk.NewFaultConfig(nil)
	err := h.Run(t, fc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
	assert.Contains(t, err.Error(), "unexpected state")
}

func TestHarness_InvariantViolation(t *testing.T) {
	h := NewHarness(mockFactory, testScheme(), testRequest(), testConfigMap())
	h.AddInvariant(ObjectExists(
		types.NamespacedName{Name: "nonexistent", Namespace: "default"},
		&corev1.ConfigMap{},
	))
	fc := sdk.NewFaultConfig(nil)
	err := h.Run(t, fc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invariant 0 violated")
}

// --- Invariant tests ---

func TestObjectExists_Found(t *testing.T) {
	scheme := testScheme()
	cm := testConfigMap()
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	inv := ObjectExists(
		types.NamespacedName{Name: "test-cm", Namespace: "default"},
		&corev1.ConfigMap{},
	)
	err := inv(context.Background(), c)
	assert.NoError(t, err)
}

func TestObjectExists_NotFound(t *testing.T) {
	scheme := testScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	inv := ObjectExists(
		types.NamespacedName{Name: "missing", Namespace: "default"},
		&corev1.ConfigMap{},
	)
	err := inv(context.Background(), c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected object")
}

func TestObjectCount_Match(t *testing.T) {
	scheme := testScheme()
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm1", Namespace: "default"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm2", Namespace: "default"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm1, cm2).Build()

	inv := ObjectCount(&corev1.ConfigMapList{}, 2, client.InNamespace("default"))
	err := inv(context.Background(), c)
	assert.NoError(t, err)
}

func TestObjectCount_Mismatch(t *testing.T) {
	scheme := testScheme()
	cm := testConfigMap()
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()

	inv := ObjectCount(&corev1.ConfigMapList{}, 5, client.InNamespace("default"))
	err := inv(context.Background(), c)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected 5 objects, got 1")
}

// --- Fuzz functions ---

func FuzzDecodeFaultConfig(f *testing.F) {
	f.Add(uint16(0), uint8(0), uint16(0))
	f.Add(uint16(0x01FF), uint8(0), uint16(65535))
	f.Add(uint16(1), uint8(5), uint16(32768))
	f.Add(uint16(0xFFFF), uint8(10), uint16(1))

	f.Fuzz(func(t *testing.T, opMask uint16, faultType uint8, intensity uint16) {
		fc := DecodeFaultConfig(opMask, faultType, intensity)
		require.NotNil(t, fc)
		require.True(t, fc.IsActive())

		for _, op := range allOperations {
			_ = fc.MaybeInject(op)
		}
	})
}

func FuzzHarnessRun(f *testing.F) {
	f.Add(uint16(0x01FF), uint8(0), uint16(32768))
	f.Add(uint16(0), uint8(3), uint16(65535))
	f.Add(uint16(1), uint8(1), uint16(0))
	f.Add(uint16(0xFFFF), uint8(255), uint16(1))

	scheme := testScheme()

	f.Fuzz(func(t *testing.T, opMask uint16, faultType uint8, intensity uint16) {
		cm := testConfigMap()
		req := testRequest()

		h := NewHarness(mockFactory, scheme, req, cm)
		// Add an invariant to exercise invariant checking under fuzz.
		h.AddInvariant(ObjectExists(
			types.NamespacedName{Name: "test-cm", Namespace: "default"},
			&corev1.ConfigMap{},
		))
		fc := DecodeFaultConfig(opMask, faultType, intensity)

		// The harness should never panic. Chaos errors are expected.
		// Non-chaos errors and invariant violations are acceptable fuzz outcomes.
		_ = h.Run(t, fc)
	})
}

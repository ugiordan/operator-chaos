package sdk

import "testing"

// TestChaos provides a test-friendly API for chaos fault injection.
type TestChaos struct {
	component string
	config    *FaultConfig
}

// NewForTest creates a TestChaos instance for use in Go tests.
// Faults are automatically cleaned up when the test completes via t.Cleanup.
func NewForTest(t *testing.T, component string) *TestChaos {
	tc := &TestChaos{
		component: component,
		config: &FaultConfig{
			Active: true,
			Faults: make(map[string]FaultSpec),
		},
	}
	t.Cleanup(func() {
		tc.config.mu.Lock()
		tc.config.Active = false
		tc.config.Faults = nil
		tc.config.mu.Unlock()
	})
	return tc
}

// Activate enables fault injection for the given operation.
func (tc *TestChaos) Activate(operation string, spec FaultSpec) {
	tc.config.mu.Lock()
	defer tc.config.mu.Unlock()
	tc.config.Faults[operation] = spec
}

// Deactivate disables fault injection for the given operation.
func (tc *TestChaos) Deactivate(operation string) {
	tc.config.mu.Lock()
	defer tc.config.mu.Unlock()
	delete(tc.config.Faults, operation)
}

// Config returns the underlying FaultConfig for use with ChaosClient or WrapReconciler.
func (tc *TestChaos) Config() *FaultConfig {
	return tc.config
}

// Component returns the component name this test chaos is configured for.
func (tc *TestChaos) Component() string {
	return tc.component
}

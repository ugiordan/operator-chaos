package sdk

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperationConstants(t *testing.T) {
	ops := []Operation{
		OpGet,
		OpList,
		OpCreate,
		OpUpdate,
		OpDelete,
		OpPatch,
		OpDeleteAllOf,
		OpReconcile,
	}
	for _, op := range ops {
		assert.NotEmpty(t, string(op), "Operation constant must be non-empty")
	}
}

func TestFaultConfigDefaults(t *testing.T) {
	cfg := &FaultConfig{}
	assert.False(t, cfg.IsActive())
	assert.Nil(t, cfg.MaybeInject("get"))
}

func TestFaultConfigNil(t *testing.T) {
	var cfg *FaultConfig
	assert.False(t, cfg.IsActive())
	assert.Nil(t, cfg.MaybeInject("get"))
}

func TestFaultConfigActive(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 1.0, Error: "simulated error"},
		},
	}
	assert.True(t, cfg.IsActive())
	err := cfg.MaybeInject("get")
	assert.Error(t, err)
	assert.Equal(t, "simulated error", err.Error())
}

func TestFaultConfigInactiveNoInjection(t *testing.T) {
	cfg := &FaultConfig{
		Active: false,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 1.0, Error: "simulated error"},
		},
	}
	assert.Nil(t, cfg.MaybeInject("get"))
}

func TestFaultConfigNoMatchingOperation(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 1.0, Error: "simulated error"},
		},
	}
	assert.Nil(t, cfg.MaybeInject("create"))
}

func TestFaultConfigPartialErrorRate(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 0.0, Error: "should not fire"},
		},
	}
	// 0% error rate should never inject
	for i := 0; i < 100; i++ {
		assert.Nil(t, cfg.MaybeInject("get"))
	}
}

func TestFaultConfigConcurrentAccess(t *testing.T) {
	cfg := &FaultConfig{
		Active: true,
		Faults: map[string]FaultSpec{
			"get": {ErrorRate: 0.5, Error: "concurrent error"},
		},
	}

	var wg sync.WaitGroup

	// Spawn 50 goroutines that each call MaybeInject("get") 100 times
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = cfg.MaybeInject("get")
			}
		}()
	}

	// Spawn 10 goroutines that toggle Active 100 times each
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cfg.mu.Lock()
				cfg.Active = !cfg.Active
				cfg.mu.Unlock()
			}
		}()
	}

	wg.Wait()
}

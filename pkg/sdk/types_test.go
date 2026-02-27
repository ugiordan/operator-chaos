package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

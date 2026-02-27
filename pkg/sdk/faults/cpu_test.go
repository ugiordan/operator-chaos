package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGoroutineBombConfig(t *testing.T) {
	spec := GoroutineBombConfig(10000)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "goroutine bomb: 10000 goroutines spawned", spec.Error)
}

func TestGoroutineBombConfigSmallCount(t *testing.T) {
	spec := GoroutineBombConfig(100)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "goroutine bomb: 100 goroutines spawned", spec.Error)
}

func TestBusySpinConfig(t *testing.T) {
	spec := BusySpinConfig(2 * time.Second)

	assert.Equal(t, 2*time.Second, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "cpu: busy spin", spec.Error)
}

func TestBusySpinConfigShortDuration(t *testing.T) {
	spec := BusySpinConfig(50 * time.Millisecond)

	assert.Equal(t, 50*time.Millisecond, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "cpu: busy spin", spec.Error)
}

func TestGCPressureConfig(t *testing.T) {
	spec := GCPressureConfig(0.6)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.6, spec.ErrorRate)
	assert.Equal(t, "gc pressure: excessive allocations", spec.Error)
}

func TestGCPressureConfigFullRate(t *testing.T) {
	spec := GCPressureConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "gc pressure: excessive allocations", spec.Error)
}

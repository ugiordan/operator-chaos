package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryLeakConfig(t *testing.T) {
	spec := MemoryLeakConfig(1024*1024, 100*time.Millisecond)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "memory leak: 1048576 bytes allocated", spec.Error)
	assert.Equal(t, 100*time.Millisecond, spec.Delay)
}

func TestMemoryLeakConfigLargeAlloc(t *testing.T) {
	spec := MemoryLeakConfig(1024*1024*512, 5*time.Second)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "memory leak: 536870912 bytes allocated", spec.Error)
	assert.Equal(t, 5*time.Second, spec.Delay)
}

func TestMemoryPressureConfig(t *testing.T) {
	spec := MemoryPressureConfig(0.7)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.7, spec.ErrorRate)
	assert.Equal(t, "memory pressure: allocation failed", spec.Error)
}

func TestMemoryPressureConfigFullRate(t *testing.T) {
	spec := MemoryPressureConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "memory pressure: allocation failed", spec.Error)
}

func TestAllocSpikeConfig(t *testing.T) {
	spec := AllocSpikeConfig(0.5, 1024*1024*100)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.5, spec.ErrorRate)
	assert.Equal(t, "allocation spike: 104857600 bytes", spec.Error)
}

func TestAllocSpikeConfigFullRate(t *testing.T) {
	spec := AllocSpikeConfig(1.0, 2048)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "allocation spike: 2048 bytes", spec.Error)
}

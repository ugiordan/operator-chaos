package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestForceErrorConfig(t *testing.T) {
	spec := ForceErrorConfig("forced failure", 0.9)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.9, spec.ErrorRate)
	assert.Equal(t, "forced failure", spec.Error)
}

func TestForceErrorConfigFullRate(t *testing.T) {
	spec := ForceErrorConfig("always fail", 1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "always fail", spec.Error)
}

func TestSkipConfig(t *testing.T) {
	spec := SkipConfig(0.5)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.5, spec.ErrorRate)
	assert.Equal(t, "reconciliation skipped by chaos", spec.Error)
}

func TestSkipConfigZeroRate(t *testing.T) {
	spec := SkipConfig(0.0)

	assert.Equal(t, 0.0, spec.ErrorRate)
	assert.Equal(t, "reconciliation skipped by chaos", spec.Error)
}

func TestPanicConfig(t *testing.T) {
	spec := PanicConfig("test crash", 0.1)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.1, spec.ErrorRate)
	assert.Equal(t, "panic: test crash", spec.Error)
}

func TestPanicConfigMessage(t *testing.T) {
	spec := PanicConfig("out of memory", 1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "panic: out of memory", spec.Error)
}

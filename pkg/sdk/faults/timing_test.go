package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDelayConfig(t *testing.T) {
	spec := DelayConfig(2 * time.Second)

	assert.Equal(t, 2*time.Second, spec.Delay)
	assert.Equal(t, 0.0, spec.ErrorRate)
	assert.Equal(t, "", spec.Error)
}

func TestDelayConfigSmallDuration(t *testing.T) {
	spec := DelayConfig(100 * time.Millisecond)

	assert.Equal(t, 100*time.Millisecond, spec.Delay)
	assert.Equal(t, 0.0, spec.ErrorRate)
	assert.Equal(t, "", spec.Error)
}

func TestJitterConfig(t *testing.T) {
	spec := JitterConfig(500 * time.Millisecond)

	assert.Equal(t, 500*time.Millisecond, spec.Delay)
	assert.Equal(t, 0.0, spec.ErrorRate)
	assert.Equal(t, "", spec.Error)
}

func TestJitterConfigLargeDuration(t *testing.T) {
	spec := JitterConfig(5 * time.Second)

	assert.Equal(t, 5*time.Second, spec.Delay)
	assert.Equal(t, 0.0, spec.ErrorRate)
}

func TestDeadlineExceedConfig(t *testing.T) {
	spec := DeadlineExceedConfig(0.7)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.7, spec.ErrorRate)
	assert.Equal(t, "context deadline exceeded", spec.Error)
}

func TestDeadlineExceedConfigFullRate(t *testing.T) {
	spec := DeadlineExceedConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "context deadline exceeded", spec.Error)
}

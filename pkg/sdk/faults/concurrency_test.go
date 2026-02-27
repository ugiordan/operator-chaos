package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeadlockInjectConfig(t *testing.T) {
	spec := DeadlockInjectConfig()

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "deadlock: resource lock contention", spec.Error)
}

func TestDeadlockInjectConfigValues(t *testing.T) {
	spec := DeadlockInjectConfig()

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.NotEmpty(t, spec.Error)
}

func TestChannelBlockConfig(t *testing.T) {
	spec := ChannelBlockConfig(5 * time.Second)

	assert.Equal(t, 5*time.Second, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "channel blocked: send/receive timeout", spec.Error)
}

func TestChannelBlockConfigShortDuration(t *testing.T) {
	spec := ChannelBlockConfig(100 * time.Millisecond)

	assert.Equal(t, 100*time.Millisecond, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "channel blocked: send/receive timeout", spec.Error)
}

func TestMutexStarvationConfig(t *testing.T) {
	spec := MutexStarvationConfig(0.8, 2*time.Second)

	assert.Equal(t, 2*time.Second, spec.Delay)
	assert.Equal(t, 0.8, spec.ErrorRate)
	assert.Equal(t, "mutex starvation: lock held too long", spec.Error)
}

func TestMutexStarvationConfigFullRate(t *testing.T) {
	spec := MutexStarvationConfig(1.0, 500*time.Millisecond)

	assert.Equal(t, 500*time.Millisecond, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "mutex starvation: lock held too long", spec.Error)
}

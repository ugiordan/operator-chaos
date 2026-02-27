package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClientThrottleConfig(t *testing.T) {
	spec := ClientThrottleConfig(500*time.Millisecond, 0.5)

	assert.Equal(t, 500*time.Millisecond, spec.Delay)
	assert.Equal(t, 0.5, spec.ErrorRate)
	assert.Equal(t, "client throttled", spec.Error)
}

func TestClientThrottleConfigZeroValues(t *testing.T) {
	spec := ClientThrottleConfig(0, 0)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.0, spec.ErrorRate)
	assert.Equal(t, "client throttled", spec.Error)
}

func TestAPIServerErrorConfig(t *testing.T) {
	spec := APIServerErrorConfig("internal server error", 0.8)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.8, spec.ErrorRate)
	assert.Equal(t, "internal server error", spec.Error)
}

func TestAPIServerErrorConfigCustomMessage(t *testing.T) {
	spec := APIServerErrorConfig("etcd timeout", 1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "etcd timeout", spec.Error)
}

func TestWatchDisconnectConfig(t *testing.T) {
	spec := WatchDisconnectConfig(0.3)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.3, spec.ErrorRate)
	assert.Equal(t, "watch channel closed", spec.Error)
}

func TestWatchDisconnectConfigFullRate(t *testing.T) {
	spec := WatchDisconnectConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "watch channel closed", spec.Error)
}

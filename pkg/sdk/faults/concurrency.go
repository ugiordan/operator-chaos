package faults

import (
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// DeadlockInjectConfig creates a fault that simulates a deadlock condition.
func DeadlockInjectConfig() sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     "deadlock: resource lock contention",
	}
}

// ChannelBlockConfig creates a fault that simulates a blocked channel for the given duration.
func ChannelBlockConfig(blockDuration time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     "channel blocked: send/receive timeout",
		Delay:     blockDuration,
	}
}

// MutexStarvationConfig creates a fault that simulates mutex starvation with a hold duration.
func MutexStarvationConfig(errorRate float64, holdDuration time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     "mutex starvation: lock held too long",
		Delay:     holdDuration,
	}
}

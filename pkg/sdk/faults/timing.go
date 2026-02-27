package faults

import (
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// DelayConfig creates a fault that adds a fixed delay.
func DelayConfig(delay time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		Delay: delay,
	}
}

// JitterConfig creates a fault that adds a random delay up to maxJitter.
func JitterConfig(maxJitter time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		Delay: maxJitter,
	}
}

// DeadlineExceedConfig creates a fault that simulates context deadline exceeded.
func DeadlineExceedConfig(rate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: rate,
		Error:     "context deadline exceeded",
	}
}

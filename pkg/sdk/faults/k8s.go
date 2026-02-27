package faults

import (
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// ClientThrottleConfig creates a fault that adds delay to K8s API calls.
func ClientThrottleConfig(delay time.Duration, rate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		Delay:     delay,
		ErrorRate: rate,
		Error:     "client throttled",
	}
}

// APIServerErrorConfig creates a fault that simulates API server errors.
func APIServerErrorConfig(errMsg string, rate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: rate,
		Error:     errMsg,
	}
}

// WatchDisconnectConfig creates a fault that simulates watch channel disconnection.
func WatchDisconnectConfig(rate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: rate,
		Error:     "watch channel closed",
	}
}

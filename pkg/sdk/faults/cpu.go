package faults

import (
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// GoroutineBombConfig creates a fault that simulates spawning excessive goroutines.
func GoroutineBombConfig(count int) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     fmt.Sprintf("goroutine bomb: %d goroutines spawned", count),
	}
}

// BusySpinConfig creates a fault that simulates a CPU busy spin for the given duration.
func BusySpinConfig(duration time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     "cpu: busy spin",
		Delay:     duration,
	}
}

// GCPressureConfig creates a fault that simulates garbage collection pressure.
func GCPressureConfig(errorRate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     "gc pressure: excessive allocations",
	}
}

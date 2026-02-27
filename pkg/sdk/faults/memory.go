package faults

import (
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// MemoryLeakConfig creates a fault that simulates a memory leak of the given size.
func MemoryLeakConfig(bytes int64, delay time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     fmt.Sprintf("memory leak: %d bytes allocated", bytes),
		Delay:     delay,
	}
}

// MemoryPressureConfig creates a fault that simulates memory pressure causing allocation failures.
func MemoryPressureConfig(errorRate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     "memory pressure: allocation failed",
	}
}

// AllocSpikeConfig creates a fault that simulates a sudden allocation spike.
func AllocSpikeConfig(errorRate float64, spikeBytes int64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     fmt.Sprintf("allocation spike: %d bytes", spikeBytes),
	}
}

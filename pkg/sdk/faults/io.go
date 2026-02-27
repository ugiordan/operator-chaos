package faults

import (
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// FDExhaustionConfig creates a fault that simulates file descriptor exhaustion.
func FDExhaustionConfig(maxFDs int) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     fmt.Sprintf("fd exhaustion: %d file descriptors open", maxFDs),
	}
}

// DiskWriteFailureConfig creates a fault that simulates disk write failures.
func DiskWriteFailureConfig(errorRate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     "disk write failure: I/O error",
	}
}

// SlowReaderConfig creates a fault that simulates slow reads with a delay.
func SlowReaderConfig(readDelay time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     "slow reader: read timeout",
		Delay:     readDelay,
	}
}

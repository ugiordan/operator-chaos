package faults

import (
	"fmt"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
)

// ConnectionPoolExhaustConfig creates a fault that simulates connection pool exhaustion.
func ConnectionPoolExhaustConfig(maxConns int) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     fmt.Sprintf("connection pool exhausted: %d connections in use", maxConns),
	}
}

// DNSFailureConfig creates a fault that simulates DNS resolution failures.
func DNSFailureConfig(errorRate float64) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: errorRate,
		Error:     "dns failure: lookup failed",
	}
}

// SocketTimeoutConfig creates a fault that simulates socket timeouts.
func SocketTimeoutConfig(timeout time.Duration) sdk.FaultSpec {
	return sdk.FaultSpec{
		ErrorRate: 1.0,
		Error:     "socket timeout: connection timed out",
		Delay:     timeout,
	}
}

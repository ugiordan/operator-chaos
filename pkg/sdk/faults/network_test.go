package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConnectionPoolExhaustConfig(t *testing.T) {
	spec := ConnectionPoolExhaustConfig(100)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "connection pool exhausted: 100 connections in use", spec.Error)
}

func TestConnectionPoolExhaustConfigLargePool(t *testing.T) {
	spec := ConnectionPoolExhaustConfig(5000)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "connection pool exhausted: 5000 connections in use", spec.Error)
}

func TestDNSFailureConfig(t *testing.T) {
	spec := DNSFailureConfig(0.5)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.5, spec.ErrorRate)
	assert.Equal(t, "dns failure: lookup failed", spec.Error)
}

func TestDNSFailureConfigFullRate(t *testing.T) {
	spec := DNSFailureConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "dns failure: lookup failed", spec.Error)
}

func TestSocketTimeoutConfig(t *testing.T) {
	spec := SocketTimeoutConfig(10 * time.Second)

	assert.Equal(t, 10*time.Second, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "socket timeout: connection timed out", spec.Error)
}

func TestSocketTimeoutConfigShortTimeout(t *testing.T) {
	spec := SocketTimeoutConfig(500 * time.Millisecond)

	assert.Equal(t, 500*time.Millisecond, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "socket timeout: connection timed out", spec.Error)
}

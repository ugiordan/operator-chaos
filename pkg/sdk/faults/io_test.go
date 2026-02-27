package faults

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFDExhaustionConfig(t *testing.T) {
	spec := FDExhaustionConfig(1024)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "fd exhaustion: 1024 file descriptors open", spec.Error)
}

func TestFDExhaustionConfigSmallLimit(t *testing.T) {
	spec := FDExhaustionConfig(64)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "fd exhaustion: 64 file descriptors open", spec.Error)
}

func TestDiskWriteFailureConfig(t *testing.T) {
	spec := DiskWriteFailureConfig(0.4)

	assert.Equal(t, time.Duration(0), spec.Delay)
	assert.Equal(t, 0.4, spec.ErrorRate)
	assert.Equal(t, "disk write failure: I/O error", spec.Error)
}

func TestDiskWriteFailureConfigFullRate(t *testing.T) {
	spec := DiskWriteFailureConfig(1.0)

	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "disk write failure: I/O error", spec.Error)
}

func TestSlowReaderConfig(t *testing.T) {
	spec := SlowReaderConfig(3 * time.Second)

	assert.Equal(t, 3*time.Second, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "slow reader: read timeout", spec.Error)
}

func TestSlowReaderConfigShortDelay(t *testing.T) {
	spec := SlowReaderConfig(200 * time.Millisecond)

	assert.Equal(t, 200*time.Millisecond, spec.Delay)
	assert.Equal(t, 1.0, spec.ErrorRate)
	assert.Equal(t, "slow reader: read timeout", spec.Error)
}

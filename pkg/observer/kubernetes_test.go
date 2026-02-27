package observer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKubernetesObserver(t *testing.T) {
	obs := NewKubernetesObserver(nil)
	assert.NotNil(t, obs)
}

func TestCheckSteadyStateEmptyChecks(t *testing.T) {
	obs := NewKubernetesObserver(nil)
	result, err := obs.CheckSteadyState(nil, nil, "test")
	assert.NoError(t, err)
	assert.True(t, result.Passed) // No checks = all passed
	assert.Equal(t, 0, result.ChecksRun)
}

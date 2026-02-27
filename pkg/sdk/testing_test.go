package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewForTest(t *testing.T) {
	ch := NewForTest(t, "model-registry")
	assert.NotNil(t, ch)

	ch.Activate("get", FaultSpec{ErrorRate: 1.0, Error: "test error"})

	err := ch.Config().MaybeInject("get")
	assert.Error(t, err)
	assert.Equal(t, "test error", err.Error())
}

func TestNewForTestAutoCleanup(t *testing.T) {
	var cfg *FaultConfig
	t.Run("inner", func(t *testing.T) {
		ch := NewForTest(t, "model-registry")
		ch.Activate("get", FaultSpec{ErrorRate: 1.0, Error: "test error"})
		cfg = ch.Config()
		// After this subtest, t.Cleanup should deactivate
	})
	// After inner subtest completes, config should be deactivated
	assert.False(t, cfg.IsActive())
}

func TestNewForTestDeactivate(t *testing.T) {
	ch := NewForTest(t, "test-component")
	ch.Activate("get", FaultSpec{ErrorRate: 1.0, Error: "error"})
	ch.Deactivate("get")

	err := ch.Config().MaybeInject("get")
	assert.Nil(t, err)
}

func TestNewForTestComponent(t *testing.T) {
	ch := NewForTest(t, "model-registry")
	assert.Equal(t, "model-registry", ch.Component())
}

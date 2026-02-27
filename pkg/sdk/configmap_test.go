package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFaultConfigFromData(t *testing.T) {
	data := map[string]string{
		"config": `{
			"active": true,
			"faults": {
				"get": {"errorRate": 0.5, "error": "timeout"}
			}
		}`,
	}

	cfg, err := ParseFaultConfigFromData(data)
	require.NoError(t, err)
	assert.True(t, cfg.Active)
	assert.Equal(t, 0.5, cfg.Faults["get"].ErrorRate)
	assert.Equal(t, "timeout", cfg.Faults["get"].Error)
}

func TestParseFaultConfigEmpty(t *testing.T) {
	cfg, err := ParseFaultConfigFromData(nil)
	require.NoError(t, err)
	assert.False(t, cfg.IsActive())
}

func TestParseFaultConfigEmptyMap(t *testing.T) {
	cfg, err := ParseFaultConfigFromData(map[string]string{})
	require.NoError(t, err)
	assert.False(t, cfg.IsActive())
}

func TestParseFaultConfigMissingKey(t *testing.T) {
	data := map[string]string{"other": "value"}
	cfg, err := ParseFaultConfigFromData(data)
	require.NoError(t, err)
	assert.False(t, cfg.IsActive())
}

func TestParseFaultConfigInvalidJSON(t *testing.T) {
	data := map[string]string{"config": "not-json"}
	_, err := ParseFaultConfigFromData(data)
	assert.Error(t, err)
}

func TestConfigMapConstants(t *testing.T) {
	assert.Equal(t, "odh-chaos-config", ChaosConfigMapName)
	assert.Equal(t, "config", ChaosConfigKey)
}

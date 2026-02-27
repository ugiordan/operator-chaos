package sdk

import (
	"encoding/json"
	"fmt"
)

const (
	// ChaosConfigMapName is the default name of the ConfigMap holding chaos configuration.
	ChaosConfigMapName = "odh-chaos-config"
	// ChaosConfigKey is the key within the ConfigMap's data that holds the JSON config.
	ChaosConfigKey = "config"
)

// ParseFaultConfigFromData parses a FaultConfig from a ConfigMap's data map.
// Returns a zero-value FaultConfig (inactive) if data is nil, empty, or missing the config key.
func ParseFaultConfigFromData(data map[string]string) (*FaultConfig, error) {
	if data == nil {
		return &FaultConfig{}, nil
	}
	configJSON, ok := data[ChaosConfigKey]
	if !ok || configJSON == "" {
		return &FaultConfig{}, nil
	}
	cfg := &FaultConfig{}
	if err := json.Unmarshal([]byte(configJSON), cfg); err != nil {
		return nil, fmt.Errorf("parsing chaos config: %w", err)
	}
	return cfg, nil
}

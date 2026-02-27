package model

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

const maxModelFileSize = 1 * 1024 * 1024 // 1 MB

// LoadKnowledge reads and parses an operator knowledge YAML file from the
// given path, returning the populated OperatorKnowledge struct.
func LoadKnowledge(path string) (*OperatorKnowledge, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() > maxModelFileSize {
		return nil, fmt.Errorf("file %s (%d bytes) exceeds maximum size of %d bytes", path, info.Size(), maxModelFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading knowledge file %s: %w", path, err)
	}

	var k OperatorKnowledge
	if err := yaml.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("parsing knowledge file %s: %w", path, err)
	}

	return &k, nil
}

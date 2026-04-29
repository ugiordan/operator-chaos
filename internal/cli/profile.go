package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// profilePaths holds the resolved paths for a named profile.
type profilePaths struct {
	KnowledgeDir string
}

// resolveProfile looks up a named profile and returns the knowledge directory.
// It searches in order:
//
//  1. profiles/<name>/knowledge/   (self-contained profile packs)
//  2. knowledge/<name>/            (built-in subdirectory convention)
//
// Returns an error if neither location exists.
func resolveProfile(name string) (*profilePaths, error) {
	if name == "" {
		return nil, fmt.Errorf("profile name must not be empty")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("profile name %q must not contain path separators or '..'", name)
	}

	// Try profiles/<name>/ first (self-contained packs)
	profileDir := filepath.Join("profiles", name)
	if info, err := os.Stat(profileDir); err == nil && info.IsDir() {
		kd := filepath.Join(profileDir, "knowledge")
		if dirExists(kd) {
			return &profilePaths{KnowledgeDir: kd}, nil
		}
		return nil, fmt.Errorf("profile %q exists at %s but contains no knowledge/ subdirectory", name, profileDir)
	}

	// Fall back to built-in subdirectory convention: knowledge/<name>/
	kd := filepath.Join("knowledge", name)
	if dirExists(kd) {
		return &profilePaths{KnowledgeDir: kd}, nil
	}

	return nil, fmt.Errorf("profile %q not found: checked profiles/%s/knowledge/ and knowledge/%s/", name, name, name)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

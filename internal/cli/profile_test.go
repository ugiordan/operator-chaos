package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfile_ProfilesDir(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	if err := os.MkdirAll(filepath.Join("profiles", "cert-manager", "knowledge"), 0750); err != nil {
		t.Fatal(err)
	}

	pp, err := resolveProfile("cert-manager")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.KnowledgeDir != filepath.Join("profiles", "cert-manager", "knowledge") {
		t.Errorf("expected profiles/cert-manager/knowledge, got %s", pp.KnowledgeDir)
	}
}

func TestResolveProfile_BuiltinSubdirectory(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	if err := os.MkdirAll(filepath.Join("knowledge", "rhoai"), 0750); err != nil {
		t.Fatal(err)
	}

	pp, err := resolveProfile("rhoai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.KnowledgeDir != filepath.Join("knowledge", "rhoai") {
		t.Errorf("expected knowledge/rhoai, got %s", pp.KnowledgeDir)
	}
}

func TestResolveProfile_NotFound(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	_, err := resolveProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
}

func TestResolveProfile_ProfilesDirPriority(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	// Create both: profiles/myop/knowledge/ and knowledge/myop/
	if err := os.MkdirAll(filepath.Join("profiles", "myop", "knowledge"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("knowledge", "myop"), 0750); err != nil {
		t.Fatal(err)
	}

	pp, err := resolveProfile("myop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pp.KnowledgeDir != filepath.Join("profiles", "myop", "knowledge") {
		t.Errorf("expected profiles/myop/knowledge (priority), got %s", pp.KnowledgeDir)
	}
}

func TestResolveProfile_EmptyProfileDir(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(orig) }()

	// Create profiles/myop/ with no knowledge/ subdirectory
	if err := os.MkdirAll(filepath.Join("profiles", "myop"), 0750); err != nil {
		t.Fatal(err)
	}

	_, err := resolveProfile("myop")
	if err == nil {
		t.Fatal("expected error for empty profile directory")
	}
}

func TestResolveProfile_PathTraversal(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"../../etc"},
		{"foo/bar"},
		{".."},
		{"foo\\bar"},
	}

	for _, tc := range tests {
		_, err := resolveProfile(tc.name)
		if err == nil {
			t.Errorf("expected error for profile name %q", tc.name)
		}
	}
}

func TestResolveProfile_EmptyName(t *testing.T) {
	_, err := resolveProfile("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

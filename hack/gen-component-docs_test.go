//go:build ignore

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractCustomSections(t *testing.T) {
	content := `# Header
some text
<!-- custom-start: notes -->
My custom notes here.
More notes.
<!-- custom-end: notes -->
middle content
<!-- custom-start: issues -->
Issue 1
<!-- custom-end: issues -->
footer`

	sections := extractCustomSections(content)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections["notes"] != "My custom notes here.\nMore notes.\n" {
		t.Errorf("notes section = %q", sections["notes"])
	}
	if sections["issues"] != "Issue 1\n" {
		t.Errorf("issues section = %q", sections["issues"])
	}
}

func TestExtractCustomSections_Empty(t *testing.T) {
	content := `before
<!-- custom-start: empty -->
<!-- custom-end: empty -->
after`

	sections := extractCustomSections(content)
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if sections["empty"] != "" {
		t.Errorf("empty section = %q, want empty string", sections["empty"])
	}
}

func TestExtractCustomSections_Malformed(t *testing.T) {
	content := `before
<!-- custom-start: orphan -->
some content without end marker
after`

	sections := extractCustomSections(content)
	if sections == nil {
		t.Fatal("expected non-nil map")
	}
	if len(sections) != 0 {
		t.Errorf("expected empty map for malformed markers, got %d entries", len(sections))
	}
}

func TestInjectCustomSections(t *testing.T) {
	content := `# Header
<!-- custom-start: notes -->
<!-- custom-end: notes -->
footer`

	sections := map[string]string{
		"notes": "Preserved content.\n",
	}
	result := injectCustomSections(content, sections)
	expected := `# Header
<!-- custom-start: notes -->
Preserved content.
<!-- custom-end: notes -->
footer`
	if result != expected {
		t.Errorf("inject result:\n%s\nwant:\n%s", result, expected)
	}
}

func TestCountResources(t *testing.T) {
	resources := []managedResource{
		{Kind: "Deployment"},
		{Kind: "ConfigMap"},
		{Kind: "Deployment"},
		{Kind: "Secret"},
		{Kind: "ConfigMap"},
		{Kind: "ConfigMap"},
	}
	counts := countResources(resources)
	if counts["Deployment"] != 2 {
		t.Errorf("Deployment count = %d, want 2", counts["Deployment"])
	}
	if counts["ConfigMap"] != 3 {
		t.Errorf("ConfigMap count = %d, want 3", counts["ConfigMap"])
	}
	if counts["Secret"] != 1 {
		t.Errorf("Secret count = %d, want 1", counts["Secret"])
	}
}

func TestLoadKnowledge(t *testing.T) {
	yamlContent := `operator:
  name: test-operator
  namespace: test-ns
  repository: https://github.com/example/repo
components:
  - name: test-component
    controller: TestController
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: test-deploy
        namespace: test-ns
    webhooks:
      - name: test.webhook
        type: mutating
        path: /mutate
    finalizers:
      - test.finalizer
    steadyState:
      checks:
        - type: conditionTrue
          apiVersion: apps/v1
          kind: Deployment
          name: test-deploy
          namespace: test-ns
          conditionType: Available
      timeout: "30s"
recovery:
  reconcileTimeout: "120s"
  maxReconcileCycles: 5
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	k, err := loadKnowledge(path)
	if err != nil {
		t.Fatalf("loadKnowledge: %v", err)
	}
	if k.Operator.Name != "test-operator" {
		t.Errorf("operator name = %q", k.Operator.Name)
	}
	if k.Operator.Namespace != "test-ns" {
		t.Errorf("namespace = %q", k.Operator.Namespace)
	}
	if len(k.Components) != 1 {
		t.Fatalf("components count = %d", len(k.Components))
	}
	c := k.Components[0]
	if c.Name != "test-component" {
		t.Errorf("component name = %q", c.Name)
	}
	if len(c.ManagedResources) != 1 {
		t.Errorf("managed resources = %d", len(c.ManagedResources))
	}
	if len(c.Webhooks) != 1 {
		t.Errorf("webhooks = %d", len(c.Webhooks))
	}
	if len(c.Finalizers) != 1 {
		t.Errorf("finalizers = %d", len(c.Finalizers))
	}
	if k.Recovery.ReconcileTimeout != "120s" {
		t.Errorf("reconcile timeout = %q", k.Recovery.ReconcileTimeout)
	}
	if k.Recovery.MaxReconcileCycles != 5 {
		t.Errorf("max reconcile cycles = %d", k.Recovery.MaxReconcileCycles)
	}
}

func TestLoadExperiments(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "test-comp")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	exp1 := `apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: test-pod-kill
spec:
  target:
    operator: test-op
    component: test-comp
  injection:
    type: PodKill
    parameters:
      labelSelector: app=test
    ttl: "300s"
  hypothesis:
    description: >-
      When the pod is killed, it should recover.
    recoveryTimeout: 60s
`
	exp2 := `apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: test-config-drift
spec:
  target:
    operator: test-op
    component: test-comp
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      key: data
    ttl: "300s"
  hypothesis:
    description: Config drift test.
    recoveryTimeout: 120s
`
	if err := os.WriteFile(filepath.Join(subDir, "pod-kill.yaml"), []byte(exp1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "config-drift.yaml"), []byte(exp2), 0644); err != nil {
		t.Fatal(err)
	}

	exps, err := loadExperiments(dir, "test-comp")
	if err != nil {
		t.Fatalf("loadExperiments: %v", err)
	}
	if len(exps) != 2 {
		t.Fatalf("expected 2 experiments, got %d", len(exps))
	}

	// Check sorted by filename
	byName := map[string]experimentInfo{}
	for _, e := range exps {
		byName[e.Filename] = e
	}

	pk := byName["pod-kill.yaml"]
	if pk.Name != "test-pod-kill" {
		t.Errorf("pod-kill name = %q", pk.Name)
	}
	if pk.InjectionType != "PodKill" {
		t.Errorf("pod-kill injection type = %q", pk.InjectionType)
	}
	if pk.DangerLevel != "low" {
		t.Errorf("pod-kill danger = %q, want low (default for PodKill)", pk.DangerLevel)
	}

	cd := byName["config-drift.yaml"]
	if cd.DangerLevel != "high" {
		t.Errorf("config-drift danger = %q, want high", cd.DangerLevel)
	}
	if cd.InjectionType != "ConfigDrift" {
		t.Errorf("config-drift injection type = %q", cd.InjectionType)
	}
}

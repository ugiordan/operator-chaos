package upgrade

import (
	"strings"
	"testing"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

// --- Task 6: Basic sequencer tests ---

func TestSequenceNoDeps(t *testing.T) {
	steps := []Step{
		{Name: "a", Component: "comp-a"},
		{Name: "b", Component: "comp-b"},
		{Name: "c", Component: "comp-c"},
	}

	result, warnings, err := Sequence(steps, nil, SequencerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result))
	}
	// Without dependencies, original order is preserved
	for i, s := range result {
		if s.Name != steps[i].Name {
			t.Errorf("step %d: expected %q, got %q", i, steps[i].Name, s.Name)
		}
	}
}

func TestSequenceExplicitDependsOn(t *testing.T) {
	steps := []Step{
		{Name: "c", DependsOn: []string{"a", "b"}, Component: "comp-c"},
		{Name: "a", Component: "comp-a"},
		{Name: "b", Component: "comp-b"},
	}

	result, warnings, err := Sequence(steps, nil, SequencerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	// a and b must come before c
	posOf := make(map[string]int)
	for i, s := range result {
		posOf[s.Name] = i
	}

	if posOf["a"] >= posOf["c"] {
		t.Errorf("expected 'a' before 'c', got positions a=%d c=%d", posOf["a"], posOf["c"])
	}
	if posOf["b"] >= posOf["c"] {
		t.Errorf("expected 'b' before 'c', got positions b=%d c=%d", posOf["b"], posOf["c"])
	}
}

func TestSequenceCycleWarning(t *testing.T) {
	steps := []Step{
		{Name: "a", DependsOn: []string{"b"}, Component: "comp-a"},
		{Name: "b", DependsOn: []string{"a"}, Component: "comp-b"},
	}

	result, warnings, err := Sequence(steps, nil, SequencerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All steps must be returned
	if len(result) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result))
	}

	// Must have a cycle warning
	if len(warnings) == 0 {
		t.Fatal("expected cycle warning, got none")
	}

	foundCycleWarning := false
	for _, w := range warnings {
		if strings.Contains(w, "cycle") {
			foundCycleWarning = true
			break
		}
	}
	if !foundCycleWarning {
		t.Errorf("expected warning containing 'cycle', got: %v", warnings)
	}
}

func TestSequenceUnknownDependsOnError(t *testing.T) {
	steps := []Step{
		{Name: "a", DependsOn: []string{"nonexistent"}, Component: "comp-a"},
	}

	_, _, err := Sequence(steps, nil, SequencerOptions{})
	if err == nil {
		t.Fatal("expected error for unknown dependsOn reference")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention the unknown step, got: %v", err)
	}
}

// --- Task 7: Health gates and auto-inference tests ---

func TestSequenceHealthGateInjection(t *testing.T) {
	steps := []Step{
		{Name: "upgrade-kserve", Component: "kserve"},
		{Name: "upgrade-llamastack", Component: "llamastack", DependsOn: []string{"upgrade-kserve"}},
	}

	result, _, err := Sequence(steps, nil, SequencerOptions{InjectHealthGates: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: upgrade-kserve, health-check-kserve, upgrade-llamastack
	if len(result) != 3 {
		t.Fatalf("expected 3 steps (with health gate), got %d: %v", len(result), stepNames(result))
	}

	if result[1].Name != "health-check-kserve" {
		t.Errorf("expected health gate at position 1, got %q", result[1].Name)
	}
	if !result[1].Synthetic {
		t.Error("health gate should be synthetic")
	}
	if result[1].Type != "validate-version" {
		t.Errorf("health gate type should be 'validate-version', got %q", result[1].Type)
	}
}

func TestSequenceNoHealthGateWhenDisabled(t *testing.T) {
	steps := []Step{
		{Name: "upgrade-kserve", Component: "kserve"},
		{Name: "upgrade-llamastack", Component: "llamastack", DependsOn: []string{"upgrade-kserve"}},
	}

	result, _, err := Sequence(steps, nil, SequencerOptions{InjectHealthGates: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 steps (no health gate), got %d: %v", len(result), stepNames(result))
	}
}

func TestSequenceNoHealthGateSameComponent(t *testing.T) {
	steps := []Step{
		{Name: "step-a", Component: "kserve"},
		{Name: "step-b", Component: "kserve", DependsOn: []string{"step-a"}},
	}

	result, _, err := Sequence(steps, nil, SequencerOptions{InjectHealthGates: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same component, no health gate injected
	if len(result) != 2 {
		t.Fatalf("expected 2 steps (same component, no health gate), got %d: %v", len(result), stepNames(result))
	}
}

func TestSequenceAutoInferWithGraph(t *testing.T) {
	// Build a graph where llamastack depends on kserve
	models := []*model.OperatorKnowledge{
		{
			Operator: model.OperatorMeta{Name: "test-operator", Namespace: "default"},
			Components: []model.ComponentModel{
				{Name: "kserve"},
				{Name: "llamastack", Dependencies: []string{"kserve"}},
			},
		},
	}

	graph, err := model.BuildDependencyGraph(models)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	// Steps without explicit dependsOn, sequencer should infer from graph
	steps := []Step{
		{Name: "upgrade-llamastack", Component: "llamastack"},
		{Name: "upgrade-kserve", Component: "kserve"},
	}

	result, _, err := Sequence(steps, graph, SequencerOptions{
		AutoInferDeps: true,
		Operators:     []string{"test-operator"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// kserve should come before llamastack because llamastack depends on kserve
	posOf := make(map[string]int)
	for i, s := range result {
		posOf[s.Name] = i
	}

	if posOf["upgrade-kserve"] >= posOf["upgrade-llamastack"] {
		t.Errorf("expected kserve before llamastack, got positions kserve=%d llamastack=%d",
			posOf["upgrade-kserve"], posOf["upgrade-llamastack"])
	}
}

// stepNames returns step names for debugging output.
func stepNames(steps []Step) []string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = s.Name
	}
	return names
}

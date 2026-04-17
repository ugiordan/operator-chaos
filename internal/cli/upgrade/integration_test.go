package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFullPlaybookExecution(t *testing.T) {
	// Create a temporary playbook
	dir := t.TempDir()
	playbookContent := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: integration-test
  description: "Integration test playbook"
upgrade:
  source:
    knowledgeDir: knowledge/v1/
    version: "1.0"
  target:
    knowledgeDir: knowledge/v2/
    version: "2.0"
  path:
    operator: test-op
    namespace: test-ns
    hops:
      - channel: stable-2.0
  steps:
    - name: pre-check
      type: mock
    - name: migrate
      type: mock
    - name: upgrade
      type: mock
    - name: chaos
      type: mock
    - name: post-check
      type: mock
`
	playbookPath := filepath.Join(dir, "playbook.yaml")
	require.NoError(t, os.WriteFile(playbookPath, []byte(playbookContent), 0644))

	// Build a registry with mock executors
	reg := NewStepRegistry()
	var stepOrder []string
	reg.Register("mock", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		stepOrder = append(stepOrder, step.Name)
		_, _ = io.WriteString(out, "  mock executed\n")
		return nil
	}})

	// Run the playbook
	pb, err := LoadPlaybook(playbookPath)
	require.NoError(t, err)

	exe := NewExecutor(reg, ExecutorOptions{StateDir: dir})

	var out bytes.Buffer
	err = exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)

	// All 5 steps executed in order
	assert.Equal(t, []string{"pre-check", "migrate", "upgrade", "chaos", "post-check"}, stepOrder)
	assert.Contains(t, out.String(), "Upgrade complete")

	// State file should be cleaned up on success
	stateFile := filepath.Join(dir, "integration-test.state.json")
	_, err = os.Stat(stateFile)
	assert.True(t, os.IsNotExist(err), "state file should be deleted on success")
}

func TestIntegrationResumeAfterFailure(t *testing.T) {
	dir := t.TempDir()
	playbookContent := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: resume-test
  description: "Resume test"
upgrade:
  source:
    knowledgeDir: knowledge/v1/
    version: "1.0"
  target:
    knowledgeDir: knowledge/v2/
    version: "2.0"
  path:
    operator: test-op
    namespace: test-ns
    hops:
      - channel: stable-2.0
  steps:
    - name: step-a
      type: mock
    - name: step-b
      type: mock
    - name: step-c
      type: mock
`
	playbookPath := filepath.Join(dir, "playbook.yaml")
	require.NoError(t, os.WriteFile(playbookPath, []byte(playbookContent), 0644))

	pb, err := LoadPlaybook(playbookPath)
	require.NoError(t, err)

	// First run: step-b fails
	reg := NewStepRegistry()
	callCount := 0
	reg.Register("mock", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		callCount++
		if step.Name == "step-b" {
			return assert.AnError
		}
		return nil
	}})

	exe := NewExecutor(reg, ExecutorOptions{StateDir: dir})
	var out bytes.Buffer
	err = exe.Run(context.Background(), pb, &out)
	assert.Error(t, err)

	// Verify state file exists with failure
	stateFile := filepath.Join(dir, "resume-test.state.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	var state PlaybookState
	require.NoError(t, json.Unmarshal(data, &state))
	assert.Equal(t, "step-b", state.FailedStep)
	assert.Contains(t, state.CompletedSteps, "step-a")

	// Second run: resume from step-b, all succeed
	reg2 := NewStepRegistry()
	var resumeSteps []string
	reg2.Register("mock", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
		resumeSteps = append(resumeSteps, step.Name)
		return nil
	}})

	exe2 := NewExecutor(reg2, ExecutorOptions{StateDir: dir, ResumeFrom: "step-b"})
	var out2 bytes.Buffer
	err = exe2.Run(context.Background(), pb, &out2)
	require.NoError(t, err)

	// step-a was skipped (in state), step-b and step-c ran
	assert.Equal(t, []string{"step-b", "step-c"}, resumeSteps)
}

// Task 15: Integration tests for full playbook lifecycle

func TestChaosPlaybookIntegration(t *testing.T) {
	dir := t.TempDir()

	// 1. Create exp1.yaml and exp2.yaml experiment files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exp1.yaml"), []byte("exp1 content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exp2.yaml"), []byte("exp2 content"), 0644))

	// 2. Write a ChaosPlaybook YAML with preflight, chaos steps, and cleanup
	playbookContent := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosPlaybook
metadata:
  name: chaos-lifecycle
  description: "Full chaos lifecycle test"
chaos:
  knowledgeDir: knowledge/chaos/
  steps:
    - name: preflight
      type: validate-version
    - name: chaos-kserve
      type: chaos
      experiments:
        - exp1.yaml
    - name: chaos-dashboard
      type: chaos
      experiments:
        - exp2.yaml
      dependsOn:
        - chaos-kserve
    - name: cleanup
      type: kubectl
      commands:
        - "odh-chaos clean"
`
	playbookPath := filepath.Join(dir, "chaos-playbook.yaml")
	require.NoError(t, os.WriteFile(playbookPath, []byte(playbookContent), 0644))

	// 3. Register mock executors for all 3 types used
	reg := NewStepRegistry()
	var executedSteps []string

	reg.Register("validate-version", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		_, _ = io.WriteString(out, "  version validated\n")
		return nil
	}})

	reg.Register("chaos", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		_, _ = fmt.Fprintf(out, "  chaos experiments: %v\n", step.Experiments)
		return nil
	}})

	reg.Register("kubectl", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		_, _ = fmt.Fprintf(out, "  kubectl commands: %v\n", step.Commands)
		return nil
	}})

	// 4. Load playbook via LoadPlaybook()
	pb, err := LoadPlaybook(playbookPath)
	require.NoError(t, err)
	assert.Equal(t, "ChaosPlaybook", pb.Kind)

	// 5. Run via Executor
	exe := NewExecutor(reg, ExecutorOptions{StateDir: dir})

	var out bytes.Buffer
	err = exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)

	// 6. Verify output contains "ChaosPlaybook:" header and step outputs
	output := out.String()
	assert.Contains(t, output, "ChaosPlaybook:")
	assert.Contains(t, output, "version validated")
	assert.Contains(t, output, "chaos experiments: [exp1.yaml]")
	assert.Contains(t, output, "chaos experiments: [exp2.yaml]")
	assert.Contains(t, output, "kubectl commands: [odh-chaos clean]")

	// 7. Verify all steps executed in dependency order
	assert.Equal(t, []string{"preflight", "chaos-kserve", "chaos-dashboard", "cleanup"}, executedSteps)
}

func TestUpgradePlaybookMultiPathIntegration(t *testing.T) {
	dir := t.TempDir()

	// 1. Write an UpgradePlaybook YAML with two operators
	playbookContent := `
apiVersion: chaos.opendatahub.io/v1alpha1
kind: UpgradePlaybook
metadata:
  name: multi-path-test
  description: "Multi-path upgrade test"
upgrade:
  source:
    knowledgeDir: knowledge/v1/
    version: "1.0"
  target:
    knowledgeDir: knowledge/v2/
    version: "2.0"
  paths:
    - name: rhods-path
      operator: rhods-operator
      namespace: redhat-ods-operator
      hops:
        - channel: stable-2.0
    - name: odh-path
      operator: odh-operator
      namespace: opendatahub
      hops:
        - channel: fast-2.0
      dependsOn:
        - rhods-path
  steps:
    - name: validate-source
      type: validate-version
    - name: upgrade-rhods
      type: olm
      pathRef: rhods-operator
    - name: upgrade-odh
      type: olm
      pathRef: odh-operator
      dependsOn:
        - upgrade-rhods
    - name: validate-target
      type: validate-version
      dependsOn:
        - upgrade-odh
`
	playbookPath := filepath.Join(dir, "multi-path-playbook.yaml")
	require.NoError(t, os.WriteFile(playbookPath, []byte(playbookContent), 0644))

	// 2. Register mock executors
	reg := NewStepRegistry()
	var executedSteps []string

	reg.Register("validate-version", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		executedSteps = append(executedSteps, step.Name)
		_, _ = io.WriteString(out, "  version validated\n")
		return nil
	}})

	// 3. For the OLM executor, use a mock that calls resolvePathRef and outputs the operator name
	reg.Register("olm", &funcExecutor{fn: func(_ context.Context, step PlaybookStep, pb *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
		executedSteps = append(executedSteps, step.Name)

		// Call resolvePathRef to verify it works
		path, err := resolvePathRef(step, pb)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(out, "  OLM upgrade: operator=%s namespace=%s channel=%s\n",
			path.Operator, path.Namespace, path.Hops[0].Channel)
		return nil
	}})

	// 4. Load and run
	pb, err := LoadPlaybook(playbookPath)
	require.NoError(t, err)
	assert.Equal(t, "UpgradePlaybook", pb.Kind)

	exe := NewExecutor(reg, ExecutorOptions{StateDir: dir})

	var out bytes.Buffer
	err = exe.Run(context.Background(), pb, &out)
	require.NoError(t, err)

	// 5. Verify both operators are referenced in output
	output := out.String()
	assert.Contains(t, output, "operator=rhods-operator")
	assert.Contains(t, output, "namespace=redhat-ods-operator")
	assert.Contains(t, output, "channel=stable-2.0")
	assert.Contains(t, output, "operator=odh-operator")
	assert.Contains(t, output, "namespace=opendatahub")
	assert.Contains(t, output, "channel=fast-2.0")

	// Verify execution order respects dependencies
	assert.Equal(t, []string{"validate-source", "upgrade-rhods", "upgrade-odh", "validate-target"}, executedSteps)
}

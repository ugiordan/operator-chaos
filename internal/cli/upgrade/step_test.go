package upgrade

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubectlExecutorSuccess(t *testing.T) {
	runner := func(_ context.Context, cmd string) (string, string, error) {
		return "applied\n", "", nil
	}
	exe := NewKubectlExecutor(runner, nil)

	step := PlaybookStep{
		Name:     "migrate",
		Type:     "kubectl",
		Commands: []string{"oc apply -f foo.yaml"},
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "running: oc apply -f foo.yaml")
}

func TestKubectlExecutorFailure(t *testing.T) {
	runner := func(_ context.Context, cmd string) (string, string, error) {
		return "", "not found\n", assert.AnError
	}
	exe := NewKubectlExecutor(runner, nil)

	step := PlaybookStep{
		Name:     "migrate",
		Type:     "kubectl",
		Commands: []string{"oc get cm nonexistent"},
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command failed")
}

func TestKubectlExecutorVerifySuccess(t *testing.T) {
	runner := func(_ context.Context, cmd string) (string, string, error) {
		return "applied\n", "", nil
	}
	checker := func(_ context.Context, apiVersion, kind, namespace, labelSelector string) (string, string, error) {
		return "my-configmap\n", "", nil
	}
	exe := NewKubectlExecutor(runner, checker)

	step := PlaybookStep{
		Name:     "migrate",
		Type:     "kubectl",
		Commands: []string{"oc apply -f foo.yaml"},
		Verify: &VerifyCondition{
			Type:          "resourceExists",
			APIVersion:    "v1",
			Kind:          "ConfigMap",
			Namespace:     "test-ns",
			LabelSelector: "app=test",
		},
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "found: my-configmap")
}

func TestKubectlExecutorVerifyFailure(t *testing.T) {
	runner := func(_ context.Context, cmd string) (string, string, error) {
		return "applied\n", "", nil
	}
	checker := func(_ context.Context, apiVersion, kind, namespace, labelSelector string) (string, string, error) {
		return "", "not found\n", assert.AnError
	}
	exe := NewKubectlExecutor(runner, checker)

	step := PlaybookStep{
		Name:     "migrate",
		Type:     "kubectl",
		Commands: []string{"oc apply -f foo.yaml"},
		Verify: &VerifyCondition{
			Type:          "resourceExists",
			APIVersion:    "v1",
			Kind:          "ConfigMap",
			Namespace:     "test-ns",
			LabelSelector: "app=test",
		},
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "verify failed")
}

func TestManualExecutorInteractive(t *testing.T) {
	input := strings.NewReader("\n") // simulate Enter
	exe := NewManualExecutor(input, false, nil)

	step := PlaybookStep{
		Name:        "confirm",
		Type:        "manual",
		Description: "Check stuff",
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Check stuff")
	assert.Contains(t, out.String(), "Press Enter")
}

func TestManualExecutorSkipWithAutoCheck(t *testing.T) {
	runner := func(_ context.Context, cmd string) (string, string, error) {
		return "ok\n", "", nil
	}
	exe := NewManualExecutor(nil, true, runner)

	step := PlaybookStep{
		Name:        "confirm",
		Type:        "manual",
		Description: "Check stuff",
		AutoCheck:   "oc get pods",
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "autoCheck passed")
}

func TestManualExecutorSkipWithoutAutoCheck(t *testing.T) {
	exe := NewManualExecutor(nil, true, nil)

	step := PlaybookStep{
		Name:        "confirm",
		Type:        "manual",
		Description: "Check stuff",
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	assert.ErrorIs(t, err, ErrStepSkipped)
	assert.Contains(t, out.String(), "WARNING: skipped")
}

func TestChaosExecutorAllResilient(t *testing.T) {
	runner := func(_ context.Context, expPath, knowledgeDir string) (string, error) {
		return "Resilient", nil
	}
	exe := NewChaosExecutor(runner)

	step := PlaybookStep{
		Name:        "chaos",
		Type:        "chaos",
		Experiments: []string{"exp1.yaml", "exp2.yaml"},
		Knowledge:   "knowledge/v3.3/",
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Resilient")
}

func TestChaosExecutorOneFailed(t *testing.T) {
	callCount := 0
	runner := func(_ context.Context, expPath, knowledgeDir string) (string, error) {
		callCount++
		if callCount == 1 {
			return "Resilient", nil
		}
		return "Failed", nil
	}
	exe := NewChaosExecutor(runner)

	step := PlaybookStep{
		Name:        "chaos",
		Type:        "chaos",
		Experiments: []string{"exp1.yaml", "exp2.yaml"},
		Knowledge:   "knowledge/v3.3/",
	}

	var out bytes.Buffer
	err := exe.Execute(context.Background(), step, &PlaybookSpec{}, nil, &out)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "1/2 experiments failed")
}

func TestStepRegistry(t *testing.T) {
	reg := NewStepRegistry()

	_, err := reg.Get("unknown")
	assert.Error(t, err)

	mock := &mockExecutor{}
	reg.Register("test", mock)

	got, err := reg.Get("test")
	require.NoError(t, err)
	assert.Equal(t, mock, got)
}

type mockExecutor struct {
	called bool
	err    error
}

func (m *mockExecutor) Execute(_ context.Context, _ PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, _ io.Writer) error {
	m.called = true
	return m.err
}

func TestResolvePathRefWithMultiplePathsAndSpecificRef(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Paths: []UpgradePath{
				{Operator: "operator-a", Namespace: "ns-a", Hops: []Hop{{Channel: "stable-1"}}},
				{Operator: "operator-b", Namespace: "ns-b", Hops: []Hop{{Channel: "stable-2"}}},
			},
		},
	}

	step := PlaybookStep{Name: "upgrade-b", Type: "olm", PathRef: "operator-b"}
	path, err := resolvePathRef(step, pb)
	require.NoError(t, err)
	assert.Equal(t, "operator-b", path.Operator)
	assert.Equal(t, "ns-b", path.Namespace)
}

func TestResolvePathRefDefaultSinglePath(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Paths: []UpgradePath{
				{Operator: "only-operator", Namespace: "ns", Hops: []Hop{{Channel: "stable"}}},
			},
		},
	}

	step := PlaybookStep{Name: "upgrade", Type: "olm"}
	path, err := resolvePathRef(step, pb)
	require.NoError(t, err)
	assert.Equal(t, "only-operator", path.Operator)
}

func TestResolvePathRefErrorMultipleNoRef(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Paths: []UpgradePath{
				{Operator: "operator-a", Namespace: "ns-a"},
				{Operator: "operator-b", Namespace: "ns-b"},
			},
		},
	}

	step := PlaybookStep{Name: "upgrade", Type: "olm"}
	_, err := resolvePathRef(step, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pathRef is required when multiple paths are defined")
}

func TestResolvePathRefNoPaths(t *testing.T) {
	pb := &PlaybookSpec{Upgrade: &UpgradeSpec{}}
	step := PlaybookStep{Name: "upgrade", Type: "olm"}
	_, err := resolvePathRef(step, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no upgrade paths defined")
}

func TestResolvePathRefNilUpgrade(t *testing.T) {
	pb := &PlaybookSpec{}
	step := PlaybookStep{Name: "upgrade", Type: "olm"}
	_, err := resolvePathRef(step, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no upgrade paths defined")
}

func TestResolvePathRefNotFound(t *testing.T) {
	pb := &PlaybookSpec{
		Upgrade: &UpgradeSpec{
			Paths: []UpgradePath{
				{Operator: "operator-a", Namespace: "ns-a"},
			},
		},
	}

	step := PlaybookStep{Name: "upgrade", Type: "olm", PathRef: "nonexistent"}
	_, err := resolvePathRef(step, pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `pathRef "nonexistent" not found`)
}

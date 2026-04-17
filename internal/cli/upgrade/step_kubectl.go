package upgrade

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// CommandRunner is a function that runs a shell command and returns stdout, stderr, error.
type CommandRunner func(ctx context.Context, command string) (stdout, stderr string, err error)

// DefaultCommandRunner runs commands via os/exec.
func DefaultCommandRunner(ctx context.Context, command string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ResourceChecker verifies a resource exists given its API details.
// Returns stdout, stderr, error. Typically runs "oc get" under the hood.
type ResourceChecker func(ctx context.Context, apiVersion, kind, namespace, labelSelector string) (string, string, error)

// DefaultResourceChecker checks resource existence via oc/kubectl.
// Uses exec.CommandContext with explicit arguments to prevent shell injection.
func DefaultResourceChecker(ctx context.Context, apiVersion, kind, namespace, labelSelector string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "oc", "get", kind, "-n", namespace, "-l", labelSelector, "--no-headers")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// KubectlExecutor runs shell commands and optionally verifies resource conditions.
type KubectlExecutor struct {
	runCommand    CommandRunner
	checkResource ResourceChecker
}

func NewKubectlExecutor(runner CommandRunner, checker ResourceChecker) *KubectlExecutor {
	if runner == nil {
		runner = DefaultCommandRunner
	}
	if checker == nil {
		checker = DefaultResourceChecker
	}
	return &KubectlExecutor{runCommand: runner, checkResource: checker}
}

func (e *KubectlExecutor) Execute(ctx context.Context, step PlaybookStep, _ *PlaybookSpec, _ *PlaybookState, out io.Writer) error {
	for _, cmd := range step.Commands {
		_, _ = fmt.Fprintf(out, "  running: %s\n", cmd)
		stdout, stderr, err := e.runCommand(ctx, cmd)
		if stdout != "" {
			_, _ = fmt.Fprintf(out, "  %s", stdout)
		}
		if err != nil {
			if stderr != "" {
				_, _ = fmt.Fprintf(out, "  stderr: %s", stderr)
			}
			return fmt.Errorf("command failed: %s: %w", cmd, err)
		}
	}

	if step.Verify != nil {
		_, _ = fmt.Fprintf(out, "  verify: %s in %s (labelSelector=%s)\n",
			step.Verify.Kind, step.Verify.Namespace, step.Verify.LabelSelector)
		stdout, stderr, err := e.checkResource(ctx, step.Verify.APIVersion, step.Verify.Kind, step.Verify.Namespace, step.Verify.LabelSelector)
		if err != nil {
			if stderr != "" {
				_, _ = fmt.Fprintf(out, "  stderr: %s", stderr)
			}
			return fmt.Errorf("verify failed: %s %s in %s: %w", step.Verify.Kind, step.Verify.LabelSelector, step.Verify.Namespace, err)
		}
		if stdout != "" {
			_, _ = fmt.Fprintf(out, "  found: %s", stdout)
		}
	}

	return nil
}

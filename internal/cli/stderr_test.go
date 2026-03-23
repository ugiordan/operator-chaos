package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWarningsGoToStderr_RunCleanNilClient verifies that when runClean is
// called with a nil client, the warning goes to stderr (not stdout).
func TestWarningsGoToStderr_RunCleanNilClient(t *testing.T) {
	// Capture stderr by temporarily replacing os.Stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	// Also capture stdout to verify nothing leaks there
	oldStdout := os.Stdout
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wOut

	ctx := context.Background()
	summary := runClean(ctx, nil, "default")

	// Restore stderr/stdout before reading
	_ = w.Close()
	_ = wOut.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var stderrBuf bytes.Buffer
	_, err = stderrBuf.ReadFrom(r)
	require.NoError(t, err)
	_ = r.Close()

	var stdoutBuf bytes.Buffer
	_, err = stdoutBuf.ReadFrom(rOut)
	require.NoError(t, err)
	_ = rOut.Close()

	assert.Equal(t, 0, summary.total(), "nil client should return zero summary")

	stderrOutput := stderrBuf.String()
	stdoutOutput := stdoutBuf.String()

	assert.True(t, strings.Contains(stderrOutput, "Warning:"),
		"warning should appear on stderr, got stderr=%q", stderrOutput)
	assert.False(t, strings.Contains(stdoutOutput, "Warning:"),
		"warning should NOT appear on stdout, got stdout=%q", stdoutOutput)
}

// TestInitOutputGoesToStdout_NotStderr verifies that the init command's YAML
// template output goes to stdout (via cmd.OutOrStdout) and not to stderr.
func TestInitOutputGoesToStdout_NotStderr(t *testing.T) {
	cmd := newInitCommand()
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	cmd.SetOut(stdoutBuf)
	cmd.SetErr(stderrBuf)
	cmd.SetArgs([]string{"--component", "dashboard"})

	require.NoError(t, cmd.Execute())

	stdoutOutput := stdoutBuf.String()
	stderrOutput := stderrBuf.String()

	// stdout should contain the YAML template
	assert.True(t, strings.Contains(stdoutOutput, "apiVersion:"),
		"stdout should contain YAML template, got stdout=%q", stdoutOutput)
	assert.True(t, strings.Contains(stdoutOutput, "ChaosExperiment"),
		"stdout should contain ChaosExperiment kind")

	// stderr should be empty (no warnings for valid init)
	assert.Empty(t, stderrOutput,
		"stderr should be empty for valid init command, got stderr=%q", stderrOutput)
}

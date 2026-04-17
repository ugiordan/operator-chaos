// internal/cli/playbook.go
package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/internal/cli/upgrade"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/olm"
	pkgupgrade "github.com/opendatahub-io/odh-platform-chaos/pkg/upgrade"
	"github.com/spf13/cobra"
)

func newPlaybookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "playbook",
		Short: "Execute upgrade and chaos playbooks",
		Long: `Run multi-step playbooks that orchestrate upgrades, chaos experiments,
and validation steps. Supports both UpgradePlaybook and ChaosPlaybook kinds.`,
	}

	cmd.AddCommand(
		newPlaybookRunCommand(),
	)

	return cmd
}

func newPlaybookRunCommand() *cobra.Command {
	var (
		playbookPath string
		resumeFrom   string
		forceResume  bool
		skipManual   bool
		allowShell   bool
		dryRun       bool
		stateDir     string
		reportDir    string
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute a playbook (UpgradePlaybook or ChaosPlaybook)",
		RunE: func(cmd *cobra.Command, args []string) error {
			pb, err := upgrade.LoadPlaybook(playbookPath)
			if err != nil {
				return err
			}

			if errs := upgrade.ValidatePlaybook(pb); len(errs) > 0 {
				for _, e := range errs {
					_, _ = fmt.Fprintf(os.Stderr, "validation error: %s\n", e)
				}
				return fmt.Errorf("playbook validation failed with %d errors", len(errs))
			}

			// Check for shell commands
			if upgrade.HasShellCommands(pb) && !allowShell {
				_, _ = fmt.Fprintln(os.Stderr, "This playbook contains kubectl steps with shell commands:")
				for _, s := range pb.Steps() {
					if s.Type == "kubectl" {
						for _, c := range s.Commands {
							_, _ = fmt.Fprintf(os.Stderr, "  [%s] %s\n", s.Name, c)
						}
					}
				}
				return fmt.Errorf("use --allow-shell to permit execution of shell commands")
			}

			// Default state dir to playbook directory
			if stateDir == "" {
				stateDir = filepath.Dir(playbookPath)
			}

			// Apply sequencer when steps have dependsOn
			if hasDependsOn(pb.Steps()) {
				if err := applySequencer(pb); err != nil {
					return fmt.Errorf("sequencing steps: %w", err)
				}
			}

			// Build step registry
			reg := upgrade.NewStepRegistry()

			var olmClient *olm.Client
			if !dryRun {
				k8sClient, k8sErr := buildUpgradeK8sClient()
				if k8sErr != nil {
					return k8sErr
				}

				olmClient = olm.NewClient(k8sClient, log.New(os.Stderr, "[olm] ", 0))

				reg.Register("validate-version", upgrade.NewValidateVersionExecutor(
					makeKnowledgeValidator(k8sClient),
				))
				reg.Register("kubectl", upgrade.NewKubectlExecutor(nil, nil))
				reg.Register("manual", upgrade.NewManualExecutor(os.Stdin, skipManual, nil))
				reg.Register("olm", upgrade.NewOLMExecutor(olmClient, nil))
				reg.Register("chaos", upgrade.NewChaosExecutor(makeExperimentRunner(reportDir)))

				// Wire rollback for UpgradePlaybooks with rollback config
				if pb.Upgrade != nil && pb.Upgrade.Rollback != nil && pb.Upgrade.Rollback.Enabled {
					rollbackMgr := pkgupgrade.NewRollbackManager(olmClient, k8sClient, stateDir, *pb.Upgrade.Rollback)
					olmExe, _ := reg.Get("olm")
					if olmStep, ok := olmExe.(*upgrade.OLMExecutor); ok {
						olmStep.SetRollbackManager(rollbackMgr)
					}
				}
			}

			exe := upgrade.NewExecutor(reg, upgrade.ExecutorOptions{
				StateDir:    stateDir,
				ResumeFrom:  resumeFrom,
				ForceResume: forceResume,
				DryRun:      dryRun,
			})

			// Wire up OLM state saver now that executor is created
			if !dryRun {
				olmExe, _ := reg.Get("olm")
				if olmStep, ok := olmExe.(*upgrade.OLMExecutor); ok {
					olmStep.SetStateSaver(exe.MakeStateSaver(pb.Metadata.Name))
				}
			}

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel func()
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			return exe.Run(ctx, pb, os.Stdout)
		},
	}

	cmd.Flags().StringVar(&playbookPath, "playbook", "", "Path to playbook YAML (required)")
	cmd.Flags().StringVar(&resumeFrom, "resume-from", "", "Resume from failed step")
	cmd.Flags().BoolVar(&forceResume, "force-resume", false, "Allow resume without state file")
	cmd.Flags().BoolVar(&skipManual, "skip-manual", false, "Use autoCheck for manual steps in CI")
	cmd.Flags().BoolVar(&allowShell, "allow-shell", false, "Allow kubectl steps with shell commands")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print execution plan without running")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "Directory for state files")
	cmd.Flags().StringVar(&reportDir, "report-dir", "", "Directory for reports")
	cmd.Flags().DurationVar(&timeout, "timeout", 60*time.Minute, "Overall timeout")
	_ = cmd.MarkFlagRequired("playbook")

	return cmd
}

// hasDependsOn returns true if any step declares a dependsOn field.
func hasDependsOn(steps []upgrade.PlaybookStep) bool {
	for _, s := range steps {
		if len(s.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// applySequencer converts playbook steps to pkg/upgrade.Step, runs the
// topological sort via pkgupgrade.Sequence, and writes the reordered
// steps back into the playbook.
func applySequencer(pb *upgrade.PlaybookSpec) error {
	steps := pb.Steps()

	// Convert to pkg/upgrade.Step for the sequencer
	seqSteps := make([]pkgupgrade.Step, len(steps))
	for i, s := range steps {
		seqSteps[i] = pkgupgrade.Step{
			Name:      s.Name,
			Type:      s.Type,
			DependsOn: s.DependsOn,
			Synthetic: s.Synthetic,
		}
	}

	sorted, warnings, err := pkgupgrade.Sequence(seqSteps, nil, pkgupgrade.SequencerOptions{})
	if err != nil {
		return err
	}

	// Print warnings to stderr
	for _, w := range warnings {
		_, _ = fmt.Fprintf(os.Stderr, "sequencer warning: %s\n", w)
	}

	// Build lookup from original steps to preserve all fields
	stepByName := make(map[string]upgrade.PlaybookStep, len(steps))
	for _, s := range steps {
		stepByName[s.Name] = s
	}

	// Rebuild the step list in sorted order, preserving all playbook fields
	reordered := make([]upgrade.PlaybookStep, 0, len(sorted))
	for _, ss := range sorted {
		if original, ok := stepByName[ss.Name]; ok {
			reordered = append(reordered, original)
		} else {
			// Synthetic step injected by the sequencer (e.g. health gates)
			reordered = append(reordered, upgrade.PlaybookStep{
				Name:      ss.Name,
				Type:      ss.Type,
				Synthetic: true,
			})
		}
	}

	// Write back into the playbook
	if pb.Upgrade != nil {
		pb.Upgrade.Steps = reordered
	} else if pb.Chaos != nil {
		pb.Chaos.Steps = reordered
	}

	return nil
}

// internal/cli/upgrade.go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/internal/cli/upgrade"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/experiment"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/olm"
	pkgupgrade "github.com/opendatahub-io/odh-platform-chaos/pkg/upgrade"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func newUpgradeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "OLM upgrade management and playbook execution",
		Long: `Manage operator upgrades via OLM. Subcommands cover the full upgrade lifecycle:
discover available channels, trigger single-hop upgrades, monitor in-progress
upgrades, and execute multi-step upgrade playbooks.`,
	}

	cmd.AddCommand(
		newUpgradeDiscoverCommand(),
		newUpgradeTriggerCommand(),
		newUpgradeMonitorCommand(),
		newUpgradeRunCommand(),
	)

	return cmd
}

func newUpgradeDiscoverCommand() *cobra.Command {
	var (
		operator  string
		namespace string
		format    string
	)

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Show available OLM channels and versions for an operator",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := buildUpgradeK8sClient()
			if err != nil {
				return err
			}

			olmClient := olm.NewClient(k8sClient, log.New(os.Stderr, "[olm] ", 0))

			channels, err := olmClient.Discover(cmd.Context(), operator, namespace)
			if err != nil {
				return fmt.Errorf("discovering channels: %w", err)
			}

			currentVersion, _ := olmClient.GetCurrentVersion(cmd.Context(), operator, namespace)

			if format == "json" {
				return printUpgradeJSON(channels)
			}

			_, _ = fmt.Fprintf(os.Stdout, "\nAvailable channels for %s:\n\n", operator)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(w, "Channel\tHead Version\tCSV")
			for _, ch := range channels {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", ch.Name, ch.HeadVersion, ch.CSVName)
			}
			_ = w.Flush()

			if currentVersion != "" {
				_, _ = fmt.Fprintf(os.Stdout, "\nCurrent subscription: installedCSV=%s\n", currentVersion)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&operator, "operator", "", "OLM operator package name (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Subscription namespace (required)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format: table, json")
	_ = cmd.MarkFlagRequired("operator")
	_ = cmd.MarkFlagRequired("namespace")

	return cmd
}

func newUpgradeTriggerCommand() *cobra.Command {
	var (
		operator  string
		namespace string
		channel   string
		timeout   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Trigger a single OLM channel hop",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := buildUpgradeK8sClient()
			if err != nil {
				return err
			}

			olmClient := olm.NewClient(k8sClient, log.New(os.Stderr, "[olm] ", 0))

			if err := olmClient.PatchChannel(cmd.Context(), operator, namespace, channel); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(os.Stdout, "Patched subscription %s/%s to channel %s\n", namespace, operator, channel)
			_, _ = fmt.Fprintln(os.Stdout, "Monitoring upgrade...")

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(cmd.Context(), timeout)
				defer cancel()
			}

			statusCh, err := olmClient.WatchUpgrade(ctx, operator, namespace, 5*time.Second)
			if err != nil {
				return err
			}

			var lastStatus olm.UpgradeStatus
			for s := range statusCh {
				lastStatus = s
				_, _ = fmt.Fprintf(os.Stdout, "  %s: %s\n", s.Phase, s.Message)
			}

			if lastStatus.Phase == olm.PhaseSucceeded {
				_, _ = fmt.Fprintln(os.Stdout, "\nUpgrade succeeded.")
				return nil
			}
			return fmt.Errorf("upgrade did not succeed (last phase: %s)", lastStatus.Phase)
		},
	}

	cmd.Flags().StringVar(&operator, "operator", "", "OLM operator package name (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Subscription namespace (required)")
	cmd.Flags().StringVar(&channel, "channel", "", "Target OLM channel (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 20*time.Minute, "Max wait for CSV ready")
	_ = cmd.MarkFlagRequired("operator")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("channel")

	return cmd
}

func newUpgradeMonitorCommand() *cobra.Command {
	var (
		operator  string
		namespace string
		timeout   time.Duration
	)

	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Watch an in-progress OLM upgrade",
		RunE: func(cmd *cobra.Command, args []string) error {
			k8sClient, err := buildUpgradeK8sClient()
			if err != nil {
				return err
			}

			olmClient := olm.NewClient(k8sClient, log.New(os.Stderr, "[olm] ", 0))

			ctx := cmd.Context()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(cmd.Context(), timeout)
				defer cancel()
			}

			_, _ = fmt.Fprintf(os.Stdout, "Monitoring %s/%s...\n", namespace, operator)

			statusCh, err := olmClient.WatchUpgrade(ctx, operator, namespace, 5*time.Second)
			if err != nil {
				return err
			}

			for s := range statusCh {
				_, _ = fmt.Fprintf(os.Stdout, "  %s: %s\n", s.Phase, s.Message)
				if s.Phase == olm.PhaseSucceeded || s.Phase == olm.PhaseFailed {
					break
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&operator, "operator", "", "OLM operator package name (required)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Subscription namespace (required)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Max watch time")
	_ = cmd.MarkFlagRequired("operator")
	_ = cmd.MarkFlagRequired("namespace")

	return cmd
}

func newUpgradeRunCommand() *cobra.Command {
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
		Short: "Execute an upgrade playbook",
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
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			return exe.Run(ctx, pb, os.Stdout)
		},
	}

	cmd.Flags().StringVar(&playbookPath, "playbook", "", "Path to upgrade playbook YAML (required)")
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

// buildUpgradeK8sClient creates a controller-runtime client from kubeconfig.
func buildUpgradeK8sClient() (client.Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig: %w", err)
	}
	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}
	return k8sClient, nil
}

// makeKnowledgeValidator creates a KnowledgeValidator that checks resources on the cluster.
func makeKnowledgeValidator(k8sClient client.Client) upgrade.KnowledgeValidator {
	return func(ctx context.Context, models []*model.OperatorKnowledge) (int, int, error) {
		pass := 0
		for _, k := range models {
			allFound := true
			for _, comp := range k.Components {
				for _, res := range comp.ManagedResources {
					u := &unstructured.Unstructured{}
					u.SetAPIVersion(res.APIVersion)
					u.SetKind(res.Kind)
					ns := res.Namespace
					if ns == "" {
						ns = k.Operator.Namespace
					}
					if err := k8sClient.Get(ctx, client.ObjectKey{Name: res.Name, Namespace: ns}, u); err != nil {
						allFound = false
						break
					}
				}
				if !allFound {
					break
				}
			}
			if allFound {
				pass++
			}
		}
		return pass, len(models), nil
	}
}

// makeExperimentRunner creates an ExperimentRunner that runs experiments via the orchestrator.
func makeExperimentRunner(reportDir string) upgrade.ExperimentRunner {
	return func(ctx context.Context, experimentPath, knowledgeDir string) (string, error) {
		deps, err := buildOrchestrator(nil, knowledgeDir, false, reportDir, false, "", false)
		if err != nil {
			return "", fmt.Errorf("building orchestrator: %w", err)
		}

		exp, err := experiment.Load(experimentPath)
		if err != nil {
			return "", fmt.Errorf("loading experiment: %w", err)
		}

		result, err := deps.Orchestrator.Run(ctx, exp)
		if err != nil {
			return "", err
		}

		return string(result.Verdict), nil
	}
}

func printUpgradeJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

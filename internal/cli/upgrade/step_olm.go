package upgrade

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/olm"
	pkgupgrade "github.com/opendatahub-io/odh-platform-chaos/pkg/upgrade"
)

// StateSaver persists playbook state to disk. Called after each hop completes.
type StateSaver func(state *PlaybookState) error

// OLMExecutor triggers OLM upgrades by walking through channel hops.
type OLMExecutor struct {
	olmClient *olm.Client
	saveState StateSaver
	rollback  *pkgupgrade.RollbackManager
}

func NewOLMExecutor(olmClient *olm.Client, saveState StateSaver) *OLMExecutor {
	return &OLMExecutor{olmClient: olmClient, saveState: saveState}
}

// SetStateSaver sets the state persistence callback. Called by the executor
// after initialization to wire up per-hop state saving.
func (e *OLMExecutor) SetStateSaver(saver StateSaver) {
	e.saveState = saver
}

// SetRollbackManager configures a RollbackManager for snapshot/restore around hops.
func (e *OLMExecutor) SetRollbackManager(mgr *pkgupgrade.RollbackManager) {
	e.rollback = mgr
}

// resolvePathRef resolves the upgrade path for an OLM step.
// If pathRef is empty and there's exactly one path, it returns that path.
// If pathRef is set, it looks up by operator name across the defined paths.
func resolvePathRef(step PlaybookStep, pb *PlaybookSpec) (*UpgradePath, error) {
	if pb.Upgrade == nil || len(pb.Upgrade.Paths) == 0 {
		return nil, fmt.Errorf("no upgrade paths defined")
	}

	paths := pb.Upgrade.Paths

	if step.PathRef == "" {
		if len(paths) == 1 {
			return &paths[0], nil
		}
		return nil, fmt.Errorf("pathRef is required when multiple paths are defined")
	}

	for i := range paths {
		if paths[i].Operator == step.PathRef {
			return &paths[i], nil
		}
	}

	return nil, fmt.Errorf("pathRef %q not found in upgrade paths", step.PathRef)
}

func (e *OLMExecutor) Execute(ctx context.Context, step PlaybookStep, pb *PlaybookSpec, state *PlaybookState, out io.Writer) error {
	path, err := resolvePathRef(step, pb)
	if err != nil {
		return fmt.Errorf("olm step %q: %w", step.Name, err)
	}

	// Determine which hops are already completed (for resume)
	completedHops := map[string]bool{}
	if state != nil {
		if sr, ok := state.CompletedSteps[step.Name]; ok {
			for _, h := range sr.CompletedHops {
				completedHops[h] = true
			}
		}
	}

	for i, hop := range path.Hops {
		if completedHops[hop.Channel] {
			_, _ = fmt.Fprintf(out, "  Hop %d/%d: %s (already completed, skipping)\n", i+1, len(path.Hops), hop.Channel)
			continue
		}

		_, _ = fmt.Fprintf(out, "  Hop %d/%d: %s\n", i+1, len(path.Hops), hop.Channel)

		// Snapshot before hop for rollback support.
		// Determine the current channel: for hop 0 this is unknown (empty string),
		// for hop N>0 it's the channel from hop N-1.
		if e.rollback != nil {
			var currentChannel string
			if i > 0 {
				currentChannel = path.Hops[i-1].Channel
			}
			if err := e.rollback.SnapshotBeforeHop(ctx, path.Operator, path.Namespace, currentChannel, i); err != nil {
				_, _ = fmt.Fprintf(out, "  warning: snapshot before hop %d failed: %v\n", i+1, err)
			}
		}

		// Patch the subscription channel
		if err := e.olmClient.PatchChannel(ctx, path.Operator, path.Namespace, hop.Channel); err != nil {
			return fmt.Errorf("hop %d (%s): patching channel: %w", i+1, hop.Channel, err)
		}

		// Monitor the upgrade
		maxWait := hop.MaxWait.Duration
		if maxWait == 0 {
			maxWait = DefaultMaxWait
		}

		hopCtx, hopCancel := context.WithTimeout(ctx, maxWait)

		pollInterval := 5 * time.Second
		statusCh, err := e.olmClient.WatchUpgrade(hopCtx, path.Operator, path.Namespace, pollInterval)
		if err != nil {
			hopCancel()
			return fmt.Errorf("hop %d (%s): starting watch: %w", i+1, hop.Channel, err)
		}

		var lastStatus olm.UpgradeStatus
		for s := range statusCh {
			lastStatus = s
			_, _ = fmt.Fprintf(out, "    %s: %s\n", s.Phase, s.Message)
		}
		hopCancel()

		hopFailed := false
		var hopErr error
		if lastStatus.Phase == olm.PhaseFailed {
			hopFailed = true
			hopErr = fmt.Errorf("hop %d (%s): upgrade failed: %s", i+1, hop.Channel, lastStatus.Message)
		} else if lastStatus.Phase != olm.PhaseSucceeded {
			hopFailed = true
			hopErr = fmt.Errorf("hop %d (%s): upgrade did not succeed (last phase: %s)", i+1, hop.Channel, lastStatus.Phase)
		}

		if hopFailed {
			if e.rollback != nil {
				_, _ = fmt.Fprintf(out, "  Attempting rollback for hop %d...\n", i+1)
				rbErr := e.rollback.RollbackHop(ctx, path.Operator, path.Namespace, i)
				if state != nil {
					if state.CompletedSteps == nil {
						state.CompletedSteps = make(map[string]StepResult)
					}
					sr := state.CompletedSteps[step.Name]
					sr.RollbackAttempted = true
					if rbErr != nil {
						sr.RollbackError = rbErr.Error()
						_, _ = fmt.Fprintf(out, "  Rollback failed: %v\n", rbErr)
					} else {
						sr.RollbackSucceeded = true
						_, _ = fmt.Fprintf(out, "  Rollback succeeded\n")
					}
					state.CompletedSteps[step.Name] = sr
					if e.saveState != nil {
						if err := e.saveState(state); err != nil {
							_, _ = fmt.Fprintf(out, "  warning: failed to persist rollback state: %v\n", err)
						}
					}
				}
			}
			return hopErr
		}

		// Record completed hop in state and persist immediately for crash recovery
		if state != nil {
			if state.CompletedSteps == nil {
				state.CompletedSteps = make(map[string]StepResult)
			}
			sr := state.CompletedSteps[step.Name]
			sr.CompletedHops = append(sr.CompletedHops, hop.Channel)
			state.CompletedSteps[step.Name] = sr
			if e.saveState != nil {
				if err := e.saveState(state); err != nil {
					_, _ = fmt.Fprintf(out, "  warning: failed to persist hop state: %v\n", err)
				}
			}
		}

		_, _ = fmt.Fprintf(out, "  Hop %d/%d: %s completed\n", i+1, len(path.Hops), hop.Channel)
	}

	return nil
}

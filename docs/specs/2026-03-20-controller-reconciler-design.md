# Controller Mode — Reconciler and Experiment Lifecycle Design Spec

**Jira:** RHOAIENG-54105 (5 SP)
**Date:** 2026-03-20
**Status:** Draft (Rev 2 — incorporates 2 rounds of architect review)

## Problem

The chaos framework currently runs experiments only via the CLI (`odh-chaos run`), which executes the full lifecycle synchronously in a single process. There is no way to submit experiments as Kubernetes custom resources and have them reconciled by a controller. This blocks GitOps-driven chaos testing, scheduled experiments via CronJobs, and integration with the Kubernetes ecosystem (events, conditions, `kubectl wait`).

## Goals

- Add a controller reconciler that watches `ChaosExperiment` CRs and drives them through the experiment lifecycle
- Extract reusable phase methods from the existing monolithic `Orchestrator.Run()` so both CLI and controller share the same logic
- Add a `controller start` subcommand to the existing `odh-chaos` binary
- Support optional knowledge loading from ConfigMaps with graceful degradation
- Handle crash recovery — controller restarts mid-experiment must resume correctly
- Maintain full backward compatibility with the CLI path

## Non-Goals

- Multi-namespace controller (single namespace per controller instance)
- Horizontal scaling (single replica, leader election handles failover)
- Admission webhooks (deferred to future iteration)
- Metrics/Prometheus endpoint (deferred)

## Architecture

### Pattern: Phase-per-Reconcile with Requeue-with-Deadline

Each reconcile handles exactly one phase transition, persists the status update via `r.Status().Update()`, and requeues. This is the standard controller-runtime pattern — predictable, debuggable, and compatible with leader election heartbeats.

**Phase state machine:**

```
Pending → SteadyStatePre → Injecting → Observing → SteadyStatePost → Evaluating → Complete
   ↓           ↓                ↓           ↓              ↓              ↓
 Aborted    Aborted          Aborted     Aborted        Aborted        Aborted
```

Each arrow is one reconcile call. The reconciler reads `.status.phase`, executes that phase's logic, updates `.status.phase` to the next value, persists via `r.Status().Update(ctx, exp)`, and returns `ctrl.Result{Requeue: true}`.

**Critical invariant:** Every phase handler MUST call `r.Status().Update(ctx, exp)` before returning. If the status update fails with a conflict error, the handler returns the error, causing controller-runtime to requeue. The phase logic is re-executed on the next reconcile, so all phase handlers must be idempotent.

**Inject + Observe: Requeue-with-Deadline (not blocking)**

Instead of blocking the reconcile goroutine for 30-180s during injection and observation, the reconciler uses a requeue-with-deadline pattern:

1. **Injecting phase:** Add finalizer, call `InjectFault()`, record `InjectionStartedAt` in status, transition to `Observing`, persist status, requeue.
2. **Observing phase:** Check if `recoveryTimeout` has elapsed since `InjectionStartedAt`. If not, `RequeueAfter: min(remainingTime, 30s)`. If yes, call `RevertFault()`, transition to `SteadyStatePost`.

This avoids goroutine starvation, preserves leader-election heartbeats, and naturally handles crash recovery (on restart, the controller sees `InjectionStartedAt` is set and resumes waiting).

**Default recovery timeout:** If `RecoveryTimeout` is zero or unset, defaults to 60s (matching the existing CLI default in `lifecycle.go:237-239`).

### Orchestrator Method Extraction

Extract phase methods from the existing monolithic `Orchestrator.Run()` (lifecycle.go:109-382). The existing `Run()` method continues calling them sequentially for CLI compatibility.

**New exported methods on `Orchestrator`:**

```go
// ValidateExperiment performs blast radius, danger level, and injection validation.
// Used by: Pending → SteadyStatePre transition.
func (o *Orchestrator) ValidateExperiment(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

// RunPreCheck executes steady-state pre-checks.
// Used by: SteadyStatePre → Injecting transition.
func (o *Orchestrator) RunPreCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, error)

// InjectFault performs fault injection and returns events.
// Used by: Injecting → Observing transition.
func (o *Orchestrator) InjectFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) ([]v1alpha1.InjectionEvent, error)

// RevertFault reverses a previously injected fault using the Injector's Revert method.
// Idempotent — safe to call multiple times (reverting an already-reverted fault is a no-op).
// Used by: Observing → SteadyStatePost transition, abort with active fault, crash recovery, and finalizer deletion.
func (o *Orchestrator) RevertFault(ctx context.Context, exp *v1alpha1.ChaosExperiment) error

// RunPostCheck executes steady-state post-checks and collateral observation via the ObservationBoard.
// Uses the full board-based pattern (steady-state + reconciliation + collateral contributors).
// Used by: SteadyStatePost → Evaluating transition.
func (o *Orchestrator) RunPostCheck(ctx context.Context, exp *v1alpha1.ChaosExperiment) (*v1alpha1.CheckResult, []observer.Finding, error)

// EvaluateExperiment renders a verdict from observation board findings.
// Uses EvaluateFromFindings internally to preserve collateral and reconciliation signals.
// Used by: Evaluating → Complete transition.
func (o *Orchestrator) EvaluateExperiment(findings []observer.Finding, hypothesis v1alpha1.HypothesisSpec) *evaluator.EvaluationResult
```

The existing `Run()` method is refactored to call these methods sequentially, preserving identical CLI behavior. The board-based evaluation is preserved in both CLI and controller paths.

### Injector Interface: Adding `Revert()`

The current `Injector` interface returns a `CleanupFunc` closure from `Inject()`. This closure is held in memory and cannot survive a controller restart. If the controller crashes between injection and cleanup, the fault is never reverted.

**Updated interface:**

```go
type Injector interface {
    Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error
    Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error)
    Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error
}
```

`Revert()` is a declarative undo — given the same injection spec and namespace, it reverses the fault. Each injector already knows what it modified (from rollback annotations, resource names in parameters, etc.), so `Revert()` reconstructs the cleanup from the spec rather than relying on captured closures.

**Idempotency requirement:** `Revert()` MUST be idempotent. Calling it when no fault is active is a no-op (returns nil). This is critical for crash recovery and the abort path.

**CLI path:** Continues using `CleanupFunc` from `Inject()` via `defer` — no behavior change.
**Controller path:** Calls `Revert()` explicitly after observation completes, on abort when phase >= `Injecting`, on crash recovery when `InjectionStartedAt` is set, and on CR deletion via finalizer.

### ChaosExperimentStatus Enhancements

New fields added to `ChaosExperimentStatus`:

```go
type ChaosExperimentStatus struct {
    Phase              ExperimentPhase    `json:"phase,omitempty"`
    Verdict            Verdict            `json:"verdict,omitempty"`
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
    StartTime          *metav1.Time       `json:"startTime,omitempty"`
    EndTime            *metav1.Time       `json:"endTime,omitempty"`
    InjectionStartedAt *metav1.Time       `json:"injectionStartedAt,omitempty"`
    SteadyStatePre     *CheckResult       `json:"steadyStatePre,omitempty"`
    SteadyStatePost    *CheckResult       `json:"steadyStatePost,omitempty"`
    InjectionLog       []InjectionEvent   `json:"injectionLog,omitempty"`
    EvaluationResult   *EvaluationSummary `json:"evaluationResult,omitempty"`
    CleanupError       string             `json:"cleanupError,omitempty"`
    // +listType=map
    // +listMapKey=type
    Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// EvaluationSummary is the CRD-embeddable evaluation result.
type EvaluationSummary struct {
    Verdict    Verdict  `json:"verdict"`
    Confidence string   `json:"confidence,omitempty"`
    Deviations []string `json:"deviations,omitempty"`
}
```

**New fields:**

| Field | Purpose |
|-------|---------|
| `ObservedGeneration` | Standard K8s convention. Set from `exp.Generation` on each reconcile. If `exp.Generation != exp.Status.ObservedGeneration` and phase is past `Pending`, the reconciler aborts (spec mutation mid-run is unsafe). |
| `InjectionStartedAt` | Timestamp when injection was applied. Used by requeue-with-deadline to compute elapsed observation time. Also used for crash recovery and re-entry detection in `reconcileInject`. |
| `EvaluationResult` | Inline evaluation summary (verdict, confidence, deviations) visible via `kubectl get`. |
| `CleanupError` | If cleanup/revert fails, error is recorded here for debugging. |
| `Conditions` | Uses `+listType=map` and `+listMapKey=type` for proper server-side apply support. |

### Lease Lock Improvements

The current `LeaseExperimentLock` has a fixed 15-min TTL with no renewal. For experiments longer than 15 minutes, the lock expires and another experiment could interfere.

**Changes:**

1. **Updated `ExperimentLock` interface:**

```go
type ExperimentLock interface {
    Acquire(ctx context.Context, operator string, experimentName string, leaseDuration time.Duration) error
    Renew(ctx context.Context, operator string, experimentName string) error
    Release(ctx context.Context, operator string, experimentName string) error
}
```

2. **`Acquire()` changes:**
   - New `leaseDuration` parameter: sets `LeaseDurationSeconds` to `max(leaseDuration, DefaultLeaseDuration)`. CLI callers pass `0` to use the default.
   - **Self-re-acquire is idempotent:** If the lease is already held by the same `experimentName`, `Acquire()` returns nil (no-op). This handles the case where `reconcilePending` succeeds in acquiring the lock but the subsequent status update fails — on the next reconcile, `reconcilePending` calls `Acquire()` again with the same experiment name.

3. **`Release()` signature change:**
   - Add `ctx` parameter (currently uses `context.Background()` internally).
   - Add `experimentName` parameter for holder verification. Only the holder can release the lock. If another experiment holds the lock, `Release()` returns an error.
   - Returns `error` (currently returns nothing).

4. **`Renew()` semantics:** Updates `AcquireTime` to now, extending the lease. Verifies holder identity before renewing. Called by the reconciler on each requeue during active phases.

5. **Lock contention handling:** When `Acquire()` finds an active (non-expired) lease held by a **different** experiment, the reconciler returns `RequeueAfter: 30s` instead of an error. This avoids error-loop backoff flooding.

**Migration plan for interface change:**

The `ExperimentLock` interface change is compilation-breaking. All implementations and callers must be updated atomically in a single commit:

| File | Change |
|------|--------|
| `pkg/safety/mutex.go` | Update interface definition |
| `pkg/safety/lease.go` | Update `Acquire()`, `Release()`, add `Renew()` |
| `pkg/safety/local.go` | Update `Acquire()` (add duration param, ignore), `Release()` (add ctx/name, ignore), add `Renew()` (no-op) |
| `pkg/safety/lease_test.go` | Update test calls |
| `pkg/orchestrator/lifecycle.go` | Update `Run()`: `o.lock.Acquire(ctx, op, name, 0)`, `defer o.lock.Release(ctx, op, name)` (discard error in defer) |
| Mock implementations in tests | Update `alwaysLockedLock`, `spyLock`, etc. |

The `defer o.lock.Release(...)` pattern in `Run()` changes to:
```go
defer func() {
    if err := o.lock.Release(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
        o.logger.Warn("lock release failed", "error", err)
    }
}()
```

### Controller Subcommand

Add `controller start` as a subcommand of the existing `odh-chaos` CLI binary:

```
odh-chaos controller start [flags]
  --namespace         Namespace to watch (required)
  --metrics-addr      Metrics bind address (default ":8080")
  --health-addr       Health probe address (default ":8081")
  --leader-elect      Enable leader election (default true)
  --knowledge-dir     Directory with knowledge YAML files (optional)
```

**Implementation:**

```go
func startController(namespace, metricsAddr, healthAddr string, leaderElect bool, knowledgeDir string) error {
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                  scheme,
        Cache: cache.Options{
            DefaultNamespaces: map[string]cache.Config{
                namespace: {},
            },
        },
        Metrics: metricsserver.Options{
            BindAddress: metricsAddr,
        },
        HealthProbeBindAddress:  healthAddr,
        LeaderElection:          leaderElect,
        LeaderElectionID:        "odh-chaos-controller",
        LeaderElectionNamespace: namespace,
    })

    reconciler := &ChaosExperimentReconciler{
        Client:       mgr.GetClient(),  // cached client from manager
        Scheme:       mgr.GetScheme(),
        Orchestrator: buildControllerOrchestrator(mgr.GetClient(), knowledgeDir),
        Registry:     buildRegistry(mgr.GetClient()),
        Lock:         safety.NewLeaseExperimentLock(mgr.GetClient(), namespace),
        Clock:        clock.RealClock{},
        Recorder:     mgr.GetEventRecorderFor("chaosexperiment-controller"),
    }

    ctrl.NewControllerManagedBy(mgr).
        For(&v1alpha1.ChaosExperiment{}).
        Complete(reconciler)

    return mgr.Start(ctrl.SetupSignalHandler())
}
```

Key: uses `mgr.GetClient()` (cached reads from informer cache) instead of `client.New()` (direct API server calls). Uses `cache.Options.DefaultNamespaces` instead of the deprecated `Namespace` field. Uses `metricsserver.Options` instead of the deprecated `MetricsBindAddress`.

### Knowledge Loading from ConfigMaps

The controller optionally loads operator knowledge from ConfigMaps in its namespace:

1. **Discovery:** On startup, list ConfigMaps with label `chaos.opendatahub.io/knowledge: "true"`.
2. **Parsing:** Each ConfigMap's `data["knowledge.yaml"]` is parsed as `OperatorKnowledge`.
3. **Graceful degradation:** If no knowledge ConfigMaps exist, the controller runs experiments without knowledge-driven features (no reconciliation observation, no collateral detection). This is identical to CLI behavior with no `--knowledge` flag.
4. **Invalid knowledge handling:** If a knowledge ConfigMap contains invalid YAML, log a warning with the ConfigMap name and skip it. Do not fail startup. Other valid knowledge ConfigMaps are still loaded.
5. **Static on startup:** Knowledge is loaded once at startup. ConfigMap changes require controller restart (no dynamic reload in this iteration).

Also supports a `--knowledge-dir` flag for loading from a mounted volume (e.g., from a ConfigMap volume mount), which uses the existing `model.LoadKnowledgeDir()`.

### Reconciler Structure

```go
type ChaosExperimentReconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    Orchestrator *orchestrator.Orchestrator
    Registry     *injection.Registry
    Lock         safety.ExperimentLock
    Clock        clock.Clock
    Recorder     record.EventRecorder
}

func (r *ChaosExperimentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    exp := &v1alpha1.ChaosExperiment{}
    if err := r.Get(ctx, req.NamespacedName, exp); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 1. DELETION CHECK — must be FIRST, before any phase-based early returns.
    // A completed/aborted CR with a finalizer still needs finalizer removal on deletion.
    if !exp.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, exp)
    }

    // 2. Skip completed/aborted experiments (only after deletion check passes)
    if exp.Status.Phase == v1alpha1.PhaseComplete || exp.Status.Phase == v1alpha1.PhaseAborted {
        return ctrl.Result{}, nil
    }

    // 3. Spec mutation detection — abort if spec changed mid-run
    if exp.Status.ObservedGeneration != 0 && exp.Generation != exp.Status.ObservedGeneration {
        r.Recorder.Event(exp, corev1.EventTypeWarning, "SpecMutated",
            "Spec changed during active experiment; aborting")
        return r.abort(ctx, exp, "spec mutated during active experiment")
    }

    // 4. Set ObservedGeneration
    exp.Status.ObservedGeneration = exp.Generation

    // 5. Renew lease on every reconcile (if past Pending)
    if exp.Status.Phase != "" && exp.Status.Phase != v1alpha1.PhasePending {
        if err := r.Lock.Renew(ctx, exp.Spec.Target.Operator, exp.Name); err != nil {
            return r.abort(ctx, exp, fmt.Sprintf("lease renewal failed: %v", err))
        }
    }

    // 6. Phase dispatch
    switch exp.Status.Phase {
    case "", v1alpha1.PhasePending:
        return r.reconcilePending(ctx, exp)
    case v1alpha1.PhaseSteadyStatePre:
        return r.reconcilePreCheck(ctx, exp)
    case v1alpha1.PhaseInjecting:
        return r.reconcileInject(ctx, exp)
    case v1alpha1.PhaseObserving:
        return r.reconcileObserve(ctx, exp)
    case v1alpha1.PhaseSteadyStatePost:
        return r.reconcilePostCheck(ctx, exp)
    case v1alpha1.PhaseEvaluating:
        return r.reconcileEvaluate(ctx, exp)
    default:
        // Unknown phase — log warning and abort to avoid silent infinite loop
        r.Recorder.Event(exp, corev1.EventTypeWarning, "UnknownPhase",
            fmt.Sprintf("Unknown phase %q; aborting", exp.Status.Phase))
        return r.abort(ctx, exp, fmt.Sprintf("unknown phase: %s", exp.Status.Phase))
    }
}
```

**Deletion handler:**

```go
func (r *ChaosExperimentReconciler) handleDeletion(ctx context.Context, exp *v1alpha1.ChaosExperiment) (ctrl.Result, error) {
    if !controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
        return ctrl.Result{}, nil
    }

    // Revert fault if one was injected
    if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
        // Record error but still remove finalizer — we cannot block GC forever
        r.Recorder.Event(exp, corev1.EventTypeWarning, "RevertFailed",
            fmt.Sprintf("Failed to revert fault during deletion: %v", err))
        exp.Status.CleanupError = err.Error()
        // Best-effort status update (object is being deleted)
        _ = r.Status().Update(ctx, exp)
    }

    // Release lock
    _ = r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name)

    controllerutil.RemoveFinalizer(exp, cleanupFinalizer)
    return ctrl.Result{}, r.Update(ctx, exp)
}
```

**Phase handlers (key logic):**

- `reconcilePending`: Call `ValidateExperiment()`, acquire lock with `leaseDuration = recoveryTimeout * 2`, set `StartTime`, emit event, transition to `SteadyStatePre`, persist status. On lock contention (different experiment), return `RequeueAfter: 30s`.
- `reconcilePreCheck`: Call `RunPreCheck()`, store result in `status.steadyStatePre`, emit event, transition to `Injecting`. If pre-check fails, abort.
- `reconcileInject`: **Re-entry guard:** if `InjectionStartedAt` is already set, skip injection and transition to `Observing` (handles status-update-failure retry). Otherwise: add finalizer via `r.Update()`, call `InjectFault()`, store events in `status.injectionLog`, set `InjectionStartedAt`, emit event, transition to `Observing`, persist status.
- `reconcileObserve`: Compute elapsed time since `InjectionStartedAt` using `r.Clock.Now()`. If `elapsed < recoveryTimeout`, return `RequeueAfter: min(remaining, 30s)`. If elapsed, call `RevertFault()`, emit event, transition to `SteadyStatePost`, persist status. If `RevertFault()` fails, record in `CleanupError`, still transition.
- `reconcilePostCheck`: Call `RunPostCheck()`, store result in `status.steadyStatePost`, emit event, transition to `Evaluating`, persist status.
- `reconcileEvaluate`: Call `EvaluateExperiment()`, set verdict, set `EndTime`, release lock (record error in `CleanupError` if fails, proceed anyway), remove finalizer, set conditions, emit event, transition to `Complete`, persist status.

**Abort helper:**

```go
func (r *ChaosExperimentReconciler) abort(ctx context.Context, exp *v1alpha1.ChaosExperiment, reason string) (ctrl.Result, error) {
    // Revert fault if we're past injection
    phase := exp.Status.Phase
    if phase == v1alpha1.PhaseInjecting || phase == v1alpha1.PhaseObserving || phase == v1alpha1.PhaseSteadyStatePost {
        if err := r.Orchestrator.RevertFault(ctx, exp); err != nil {
            exp.Status.CleanupError = err.Error()
        }
    }

    // Release lock (best-effort)
    _ = r.Lock.Release(ctx, exp.Spec.Target.Operator, exp.Name)

    // Remove finalizer if present
    if controllerutil.ContainsFinalizer(exp, cleanupFinalizer) {
        controllerutil.RemoveFinalizer(exp, cleanupFinalizer)
        if err := r.Update(ctx, exp); err != nil {
            return ctrl.Result{}, err
        }
    }

    exp.Status.Phase = v1alpha1.PhaseAborted
    exp.Status.EndTime = &metav1.Time{Time: r.Clock.Now()}
    meta.SetStatusCondition(&exp.Status.Conditions, metav1.Condition{
        Type:    v1alpha1.ConditionComplete,
        Status:  metav1.ConditionTrue,
        Reason:  "ExperimentAborted",
        Message: reason,
    })

    r.Recorder.Event(exp, corev1.EventTypeWarning, "Aborted", reason)
    return ctrl.Result{}, r.Status().Update(ctx, exp)
}
```

### Crash Recovery

The requeue-with-deadline pattern and `Revert()` method enable natural crash recovery:

| Crash Point | Recovery Behavior |
|-------------|-------------------|
| Before injection | Phase is `Pending` or `SteadyStatePre`. Reconciler re-enters the phase, idempotent. `Acquire()` is self-re-acquire safe. |
| During injection | `InjectFault()` may have partially applied. If `InjectionStartedAt` is not set (status update failed), reconciler retries injection. Injectors must handle partial state (e.g., rollback annotations already exist). |
| After injection, status update failed | `InjectionStartedAt` not set in status. `reconcileInject` re-entry guard checks `InjectionStartedAt` — not set, so re-injects. Injectors must be idempotent for this case. |
| During observation | `InjectionStartedAt` is set, phase is `Observing`. Reconciler resumes waiting. If timeout already elapsed, proceeds to revert. |
| During cleanup/revert | Phase is `Observing` or `SteadyStatePost`. `RevertFault()` is idempotent — reverting an already-reverted fault is a no-op. |
| After evaluation | Phase is `Complete`. No action needed. |

**Injector idempotency note:** Most injectors (ConfigDrift, WebhookDisrupt, RBACRevoke, CRDMutation, FinalizerBlock, ClientFault) are naturally idempotent because they use `CreateOrUpdate` or check for existing state. `PodKill` is inherently non-idempotent (killing pods twice kills different pods), but the re-entry guard (`InjectionStartedAt` check in `reconcileInject`) prevents double injection in the controller path. In the CLI path, `Run()` calls `InjectFault()` exactly once.

### Finalizer for Cleanup Guarantee

The reconciler adds a finalizer (`chaos.opendatahub.io/cleanup`) during `reconcileInject`, **before** calling `InjectFault()`. This ensures that if the CR is deleted mid-experiment, the reconciler gets a chance to revert the fault before the CR is garbage collected.

**Lifecycle:**

1. **Added:** In `reconcileInject`, before `InjectFault()` — via `r.Update(ctx, exp)` (metadata write).
2. **Removed:** In `reconcileEvaluate`, after setting verdict — via `r.Update(ctx, exp)` (metadata write), then status update.
3. **Deletion path:** In `handleDeletion()`, after `RevertFault()` and lock release.
4. **Abort path:** In `abort()`, after `RevertFault()` and lock release.

**Important:** The deletion check in `Reconcile()` is placed BEFORE the "skip completed/aborted" early return. This ensures that even a `Complete` or `Aborted` CR with a lingering finalizer (edge case: status update succeeded but finalizer removal failed) can still be deleted.

### Clock Interface for Testability

```go
// pkg/clock/clock.go
type Clock interface {
    Now() time.Time
}

type RealClock struct{}
func (RealClock) Now() time.Time { return time.Now() }
```

The reconciler uses `r.Clock.Now()` instead of `time.Now()`. Tests inject a fake clock to control time-dependent behavior (observation timeout, lease expiry).

### Event Recording

The reconciler emits Kubernetes events on all phase transitions, failures, and aborts:

| Event | Type | Reason | When |
|-------|------|--------|------|
| Phase transition | Normal | `PhaseTransition` | Each successful phase transition |
| Pre-check failed | Warning | `PreCheckFailed` | Steady-state baseline not met |
| Injection applied | Normal | `FaultInjected` | After successful `InjectFault()` |
| Fault reverted | Normal | `FaultReverted` | After successful `RevertFault()` |
| Revert failed | Warning | `RevertFailed` | `RevertFault()` returned error |
| Experiment complete | Normal | `ExperimentComplete` | Verdict rendered |
| Experiment aborted | Warning | `Aborted` | Any abort path |
| Lock contention | Normal | `LockContention` | Requeuing due to held lease |
| Spec mutated | Warning | `SpecMutated` | Generation changed mid-run |
| Unknown phase | Warning | `UnknownPhase` | Unrecognized phase value |

## Testing Strategy

### Unit Tests (fake client)

| Test Case | What It Verifies |
|-----------|-----------------|
| Phase transitions (7 tests) | Each phase handler transitions to correct next phase, calls `Status().Update()` |
| Phase idempotency | Re-entering same phase after status update failure produces same result |
| Re-execution after status failure | Phase logic ran, status update failed, same phase re-executed — no double side effects (specifically: `reconcileInject` re-entry guard with `InjectionStartedAt`) |
| Spec mutation mid-run | `Generation != ObservedGeneration` triggers abort |
| Pre-check failure | Transitions to `Aborted`, not `Injecting` |
| Lock contention | Returns `RequeueAfter: 30s`, not error |
| Lock self-re-acquire | `Acquire()` with same experiment name is a no-op |
| Observation timeout | After `recoveryTimeout`, calls `RevertFault()`, transitions to `SteadyStatePost` |
| Observation in progress | Before timeout, returns `RequeueAfter: min(remaining, 30s)` |
| Zero recovery timeout | Defaults to 60s, does not immediately skip observation |
| Crash recovery: mid-observe | `InjectionStartedAt` set, resumes waiting |
| Crash recovery: mid-inject | `InjectionStartedAt` not set, retries injection |
| Deletion during Observing | Calls `RevertFault()`, releases lock, removes finalizer |
| Deletion during SteadyStatePre | No finalizer present, CR deleted immediately |
| Deletion during Injecting | Calls `RevertFault()`, releases lock, removes finalizer |
| Deletion of Complete CR | No finalizer present, CR deleted immediately |
| RevertFault failure during deletion | Records `CleanupError`, still removes finalizer, emits warning event |
| Finalizer lifecycle | Added at inject (before `InjectFault`), removed at complete |
| Status update conflicts | Returns error, controller-runtime requeues |
| Concurrent CRs | Multiple experiments compete for lease correctly |
| TTL expiry | Expired lease is reclaimed |
| Lease renewal | Each reconcile past Pending calls `Renew()` |
| CleanupError recording | Failed revert records error in status |
| Release failure | Records in `CleanupError`, proceeds to Complete |
| Dry run | Transitions directly to Complete with Inconclusive |
| No knowledge | Works without knowledge ConfigMaps |
| Unknown phase | Aborts with warning event |
| Abort with active fault | `abort()` calls `RevertFault()` when phase >= Injecting |
| Abort without fault | `abort()` skips `RevertFault()` when phase is Pending/SteadyStatePre |

### Integration Tests (envtest)

Two integration tests using envtest (real etcd + API server, no kubelet):

**Test 1: Happy path**
1. Install CRD
2. Create `ChaosExperiment` CR
3. Verify reconciler drives it through all phases to `Complete`
4. Verify status fields are set correctly (ObservedGeneration, StartTime, EndTime, InjectionStartedAt, Verdict)
5. Verify finalizer lifecycle (added at inject, removed at complete)
6. Verify conditions are set

**Test 2: Deletion during active experiment**
1. Create CR, let it reach `Observing` phase
2. Delete the CR
3. Verify `RevertFault()` is called (via mock)
4. Verify finalizer is removed
5. Verify CR is garbage collected

### CLI Regression

All existing CLI tests must pass after the interface changes. Specific migration needed:

| Test File | Change |
|-----------|--------|
| `pkg/safety/lease_test.go` | Update `Acquire()` calls to include `leaseDuration` param |
| `pkg/orchestrator/lifecycle_test.go` | Update mock lock implementations (`alwaysLockedLock`, `spyLock`) to new interface |
| Any test with `ExperimentLock` mocks | Add `Renew()` method, update `Release()` signature |

## File Changes Summary

| File | Change |
|------|--------|
| `api/v1alpha1/types.go` | ADD `ObservedGeneration`, `InjectionStartedAt`, `EvaluationSummary`, `CleanupError` to status; ADD `+listType=map` marker on Conditions |
| `api/v1alpha1/zz_generated.deepcopy.go` | REGENERATE |
| `config/crd/bases/chaos.opendatahub.io_chaosexperiments.yaml` | REGENERATE |
| `pkg/injection/engine.go` | ADD `Revert()` to `Injector` interface |
| `pkg/injection/*.go` | ADD `Revert()` implementation to each injector (8 injectors) |
| `pkg/safety/mutex.go` | UPDATE `ExperimentLock` interface: add `Renew()`, update `Acquire()` and `Release()` signatures |
| `pkg/safety/lease.go` | ADD `Renew()`, update `Acquire()` with duration+self-re-acquire, update `Release()` with ctx+holder verification |
| `pkg/safety/local.go` | UPDATE to match new interface (no-op implementations) |
| `pkg/safety/lease_test.go` | UPDATE test calls for new signatures |
| `pkg/orchestrator/lifecycle.go` | EXTRACT phase methods, refactor `Run()` to call them, update lock calls |
| `pkg/orchestrator/lifecycle_test.go` | UPDATE mock lock implementations |
| `pkg/clock/clock.go` | NEW — `Clock` interface and `RealClock` |
| `internal/controller/reconciler.go` | NEW — `ChaosExperimentReconciler` with phase handlers, deletion, abort |
| `internal/controller/reconciler_test.go` | NEW — unit tests with fake client (~28 test cases) |
| `internal/controller/suite_test.go` | NEW — envtest integration tests (2 tests) |
| `internal/cli/controller.go` | NEW — `controller start` subcommand |
| `internal/cli/orchestrator.go` | UPDATE — extract shared builder logic |
| `go.mod` / `go.sum` | UPDATE — add `sigs.k8s.io/controller-runtime` (already indirect dep) |

## Backward Compatibility

- `Orchestrator.Run()` continues to work identically for CLI users
- `CleanupFunc` from `Inject()` remains — CLI continues using it via `defer`
- New status fields are `omitempty` — existing experiment YAMLs are unaffected
- `Revert()` is additive to the `Injector` interface — existing injectors gain a new method
- **Breaking interface changes** (coordinated in single commit):
  - `ExperimentLock.Acquire()`: adds `leaseDuration time.Duration` parameter — CLI passes `0` for default
  - `ExperimentLock.Release()`: adds `ctx`, `experimentName` parameters, returns `error` — CLI wraps in error-discarding defer
  - `ExperimentLock` gains `Renew()` method — `localExperimentLock` implements as no-op
  - All mock implementations in tests must be updated

## Dependencies

- `sigs.k8s.io/controller-runtime` v0.23.1 (already indirect dependency via `controller-gen`)
- `k8s.io/client-go` v0.35.1 (already in go.mod)
- No new external dependencies

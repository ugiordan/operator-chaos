# Observer Blackboard Pattern

The observation system uses the **Blackboard architectural pattern** to collect evidence from multiple independent observers. This enables holistic evaluation of experiment outcomes by combining reconciliation metrics, steady-state checks, and collateral damage detection.

## What is the Blackboard Pattern?

The Blackboard pattern is a software architecture where multiple specialized "knowledge sources" (observers) contribute information to a shared data structure (blackboard). Each observer:

1. Reads the current state from the blackboard
2. Performs its specialized analysis
3. Writes findings back to the blackboard
4. Does NOT directly communicate with other observers

This decouples observers and makes the system extensible — new observers can be added without modifying existing ones.

## Why Use It for Chaos Engineering?

Chaos experiments require **multi-dimensional observation**:

- Did the target operator recover? (Reconciliation)
- Did the system return to baseline? (Steady-state)
- Did other components break? (Collateral damage)

Traditional single-observer systems miss the full picture. The blackboard pattern allows:

- **Parallel observation** — Multiple observers run concurrently
- **Incremental evidence** — Observers contribute at different times
- **Holistic evaluation** — Evaluator sees all evidence, not just one observer's view

## Architecture

```mermaid
graph TB
    subgraph "Orchestrator"
        Orchestrator[Lifecycle Orchestrator]
    end

    subgraph "Blackboard"
        Board[ObservationBoard<br/>Shared Data Structure]
    end

    subgraph "Phase 1: Reconciliation"
        Recon[ReconciliationContributor]
    end

    subgraph "Phase 2: Steady-State & Collateral"
        Steady[SteadyStateContributor]
        Collateral[CollateralContributor]
    end

    subgraph "Evaluation"
        Evaluator[Evaluator]
    end

    Orchestrator -->|1. Create board| Board
    Orchestrator -->|2. Run Phase 1| Recon
    Recon -->|3. Write findings| Board
    Orchestrator -->|4. Run Phase 2| Steady
    Orchestrator -->|4. Run Phase 2| Collateral
    Steady -->|5. Write findings| Board
    Collateral -->|5. Write findings| Board
    Board -->|6. Read all findings| Evaluator
```

## Core Components

### ObservationBoard

Thread-safe data structure for storing findings:

```go
type ObservationBoard struct {
    mu       sync.Mutex
    findings []Finding
}

func (b *ObservationBoard) AddFinding(f Finding) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.findings = append(b.findings, f)
}

func (b *ObservationBoard) Findings() []Finding {
    b.mu.Lock()
    defer b.mu.Unlock()
    return append([]Finding(nil), b.findings...)  // Return copy
}
```

**Key Features:**

- Thread-safe for concurrent writes
- Returns defensive copies to prevent external mutation
- Supports filtering by finding source

### Finding

Structured evidence unit:

```go
type Finding struct {
    Source               FindingSource          // reconciliation, steady_state, collateral
    Component            string                 // Target component
    Operator             string                 // Target operator
    Passed               bool                   // Did observation pass?
    Details              string                 // Human-readable details
    Checks               *v1alpha1.CheckResult  // Steady-state check results
    ReconciliationResult *ReconciliationResult  // Reconciliation metrics
}
```

**Finding Sources:**

```go
const (
    SourceReconciliation FindingSource = "reconciliation"
    SourceSteadyState    FindingSource = "steady_state"
    SourceCollateral     FindingSource = "collateral"
)
```

### ObservationContributor

Interface for all observers:

```go
type ObservationContributor interface {
    Observe(ctx context.Context, board *ObservationBoard) error
}
```

**Contract:**

- Perform observation (watch resources, run checks, etc.)
- Write findings to board via `board.AddFinding(finding)`
- Return error if observation fails (does NOT cancel other contributors)

## Observation Contributors

### 1. ReconciliationContributor

Monitors the target operator's reconciliation behavior during recovery.

**What It Observes:**

- Count of reconcile cycles triggered
- Duration of reconciliation window
- Whether reconciliation stabilized or thrashed

**Implementation:**

```go
type ReconciliationContributor struct {
    checker   ReconciliationCheckerInterface
    component *model.Component
    namespace string
    timeout   time.Duration
}

func (r *ReconciliationContributor) Observe(ctx context.Context, board *ObservationBoard) error {
    startTime := time.Now()
    result, err := r.checker.WaitForReconciliation(ctx, r.component, r.namespace, r.timeout)
    if err != nil {
        return fmt.Errorf("reconciliation check failed: %w", err)
    }

    board.AddFinding(Finding{
        Source:               SourceReconciliation,
        Component:            r.component.Name,
        Operator:             r.component.Operator,
        Passed:               result.Cycles > 0,
        ReconciliationResult: result,
    })

    return nil
}
```

**Reconciliation Result:**

```go
type ReconciliationResult struct {
    Cycles         int           // Number of reconcile events observed
    Duration       time.Duration // Time to stabilization
    FirstEventTime time.Time     // First reconcile event
    LastEventTime  time.Time     // Last reconcile event
}
```

**Evaluation:**

- **< 3 cycles** → `DEGRADED` (slow or failed recovery)
- **3-10 cycles** → `RESILIENT` (normal recovery)
- **> 10 cycles** → `DEGRADED` (thrashing)

### 2. SteadyStateContributor

Runs user-defined checks to verify system baseline.

**What It Observes:**

- Resource existence (`CheckResourceExists`)
- Condition status (`CheckConditionTrue`)

**Implementation:**

```go
type SteadyStateContributor struct {
    observer  Observer
    checks    []v1alpha1.SteadyStateCheck
    namespace string
}

func (s *SteadyStateContributor) Observe(ctx context.Context, board *ObservationBoard) error {
    checkResult, err := s.observer.CheckSteadyState(ctx, s.checks, s.namespace)
    if err != nil {
        return fmt.Errorf("steady-state check failed: %w", err)
    }

    board.AddFinding(Finding{
        Source: SourceSteadyState,
        Passed: checkResult.Passed,
        Checks: checkResult,
    })

    return nil
}
```

**Check Types:**

```yaml
steadyState:
  checks:
    - type: resourceExists
      kind: Deployment
      name: my-operator
      namespace: opendatahub

    - type: conditionTrue
      kind: DataScienceCluster
      name: default
      conditionType: Ready
```

**CheckResult:**

```go
type CheckResult struct {
    Passed       bool          // Overall pass/fail
    ChecksRun    int32         // Total checks executed
    ChecksPassed int32         // Checks that passed
    Details      []CheckDetail // Per-check results
    Timestamp    metav1.Time   // When check ran
}

type CheckDetail struct {
    Check  SteadyStateCheck
    Passed bool
    Value  string  // Actual value observed
    Error  string  // Error message if failed
}
```

### 3. CollateralContributor

Checks dependent components for cascading failures.

**What It Observes:**

- Steady-state of components that depend on the experiment target
- Uses dependency graph to identify dependents

**Implementation:**

```go
type CollateralContributor struct {
    observer   Observer
    dependents []model.ComponentRef
}

func (c *CollateralContributor) Observe(ctx context.Context, board *ObservationBoard) error {
    for _, dep := range c.dependents {
        // Derive steady-state checks for dependent component
        checks := deriveChecksForComponent(dep)
        result, err := c.observer.CheckSteadyState(ctx, checks, dep.Namespace)

        board.AddFinding(Finding{
            Source:    SourceCollateral,
            Component: dep.Component,
            Operator:  dep.Operator,
            Passed:    result.Passed && err == nil,
            Checks:    result,
        })
    }

    return nil
}
```

**Example:**

```
Target: model-controller
Dependents: [dashboard, notebook-controller]

CollateralContributor checks:
1. Is dashboard's Deployment Ready?
2. Is notebook-controller's StatefulSet Ready?
```

**Evaluation:**

- If any dependent fails → `DEGRADED` (collateral damage detected)

## Execution Phases

The orchestrator runs contributors in two phases:

### Phase 1: Reconciliation (Blocking)

```go
board := observer.NewObservationBoard()

reconContributor := observer.NewReconciliationContributor(
    reconciler,
    component,
    namespace,
    recoveryTimeout,
)

if err := reconContributor.Observe(ctx, board); err != nil {
    log.Warn("reconciliation contributor error", "error", err)
}
```

**Why Blocking?**

Reconciliation observation must complete before steady-state checks run. We need to wait for the recovery window to elapse to accurately count reconcile cycles.

### Phase 2: Steady-State & Collateral (Concurrent)

```go
var contributors []observer.ObservationContributor

// Steady-state contributor
contributors = append(contributors, observer.NewSteadyStateContributor(
    observer, checks, namespace))

// Collateral contributor (if dependents exist)
if len(dependents) > 0 {
    contributors = append(contributors, observer.NewCollateralContributor(
        observer, dependents))
}

// Run all contributors concurrently
errs := observer.RunContributors(ctx, board, contributors)
for _, err := range errs {
    log.Warn("contributor error", "error", err)
}
```

**Why Concurrent?**

Steady-state and collateral checks are independent and can run in parallel, reducing total observation time.

### RunContributors Implementation

```go
func RunContributors(ctx context.Context, board *ObservationBoard, contributors []ObservationContributor) []error {
    var (
        wg   sync.WaitGroup
        mu   sync.Mutex
        errs []error
    )

    for _, c := range contributors {
        wg.Add(1)
        go func(contrib ObservationContributor) {
            defer wg.Done()
            if err := contrib.Observe(ctx, board); err != nil {
                mu.Lock()
                errs = append(errs, err)
                mu.Unlock()
            }
        }(c)
    }

    wg.Wait()
    return errs
}
```

**Key Design Decision:**

Errors from one contributor do NOT cancel other contributors. Each observer runs to completion, maximizing evidence collection.

## Evaluation from Findings

The evaluator consumes all findings to compute a verdict:

```go
func (e *Evaluator) EvaluateFromFindings(findings []Finding, hypothesis HypothesisSpec) *EvaluationResult {
    var (
        reconPassed   bool
        steadyPassed  bool
        collateralOK  = true
        reconCycles   int
        recoveryTime  time.Duration
    )

    for _, f := range findings {
        switch f.Source {
        case SourceReconciliation:
            reconPassed = f.Passed
            if f.ReconciliationResult != nil {
                reconCycles = f.ReconciliationResult.Cycles
                recoveryTime = f.ReconciliationResult.Duration
            }

        case SourceSteadyState:
            steadyPassed = f.Passed

        case SourceCollateral:
            if !f.Passed {
                collateralOK = false
            }
        }
    }

    // Decision tree
    if !steadyPassed {
        return &EvaluationResult{Verdict: Failed, Confidence: "high"}
    }
    if !collateralOK {
        return &EvaluationResult{Verdict: Degraded, Confidence: "medium", Deviations: []string{"collateral damage"}}
    }
    if reconCycles < 3 {
        return &EvaluationResult{Verdict: Degraded, Confidence: "low", Deviations: []string{"slow recovery"}}
    }
    return &EvaluationResult{Verdict: Resilient, Confidence: "high"}
}
```

## Extensibility

Adding a new contributor is straightforward:

### 1. Define the Contributor

```go
type MetricsContributor struct {
    promClient prometheus.Client
    queries    []string
}

func (m *MetricsContributor) Observe(ctx context.Context, board *ObservationBoard) error {
    for _, query := range m.queries {
        result, err := m.promClient.Query(ctx, query)
        if err != nil {
            return err
        }

        board.AddFinding(Finding{
            Source:  "metrics",
            Passed:  result.Value > threshold,
            Details: fmt.Sprintf("metric %q: %v", query, result.Value),
        })
    }
    return nil
}
```

### 2. Register with Orchestrator

```go
contributors = append(contributors, &MetricsContributor{
    promClient: promClient,
    queries:    []string{"rate(http_requests_total[5m])"},
})

observer.RunContributors(ctx, board, contributors)
```

### 3. Update Evaluator (Optional)

```go
for _, f := range findings {
    if f.Source == "metrics" && !f.Passed {
        deviations = append(deviations, "performance degradation")
    }
}
```

## Benefits of the Blackboard Pattern

1. **Decoupling** — Observers don't know about each other
2. **Incremental Evidence** — Findings accumulate over time
3. **Parallel Execution** — Contributors run concurrently
4. **Extensibility** — New observers added without modifying existing code
5. **Fault Isolation** — One observer's failure doesn't crash others
6. **Holistic View** — Evaluator sees all evidence, not fragmented results

## Real-World Example

```go
// Experiment: Kill model-controller pod
board := observer.NewObservationBoard()

// Phase 1: Wait for reconciliation
reconContributor.Observe(ctx, board)
// Writes: {Source: reconciliation, Passed: true, Cycles: 5, Duration: 12s}

// Phase 2: Check baseline + dependents
observer.RunContributors(ctx, board, []ObservationContributor{
    steadyStateContributor,  // Writes: {Source: steady_state, Passed: true}
    collateralContributor,   // Writes: {Source: collateral, Component: "dashboard", Passed: false}
})

findings := board.Findings()
// [
//   {Source: reconciliation, Passed: true, Cycles: 5},
//   {Source: steady_state, Passed: true},
//   {Source: collateral, Component: "dashboard", Passed: false}
// ]

result := evaluator.EvaluateFromFindings(findings, hypothesis)
// Verdict: DEGRADED (collateral damage on dashboard)
```

## Next Steps

- [Injection Engine](injection-engine.md) — How faults are executed
- [Architecture Overview](overview.md) — Full system design
- [Contributing: Development Setup](../contributing/development-setup.md) — Build from source

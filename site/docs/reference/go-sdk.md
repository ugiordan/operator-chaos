# Go SDK Reference

The ODH Platform Chaos Go SDK provides programmatic access to chaos engineering capabilities, enabling developers to build fault-tolerant operators with built-in resilience testing.

## Overview

The SDK consists of three main packages:

- **`pkg/sdk`** — Core client wrapper with fault injection capabilities
- **`pkg/sdk/faults`** — Declarative fault injection primitives (CPU, memory, network, I/O)
- **`pkg/sdk/fuzz`** — Property-based testing harness for operator reconciliation

## SDK Package (`pkg/sdk`)

[:octicons-link-external-16: View on pkg.go.dev](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-chaos/pkg/sdk)

### ChaosClient

`ChaosClient` wraps a standard `controller-runtime` client with optional fault injection. It implements the full `client.Client` interface, making it a drop-in replacement for your operator's Kubernetes client.

```go
import (
    "github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Create a fault configuration
faults := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
    sdk.OpGet: {
        ErrorRate: 0.1,        // 10% of Get calls fail
        Error:     "simulated API server timeout",
    },
    sdk.OpUpdate: {
        Delay:     50 * time.Millisecond,  // Fixed delay
        MaxDelay:  200 * time.Millisecond, // Or random delay up to 200ms
    },
})

// Wrap your existing client
innerClient := ... // your controller-runtime client
chaosClient := sdk.NewChaosClient(innerClient, faults)

// Use it exactly like a normal client
pod := &corev1.Pod{}
err := chaosClient.Get(ctx, key, pod)  // May inject faults based on config
```

**Key Methods:**

- `NewChaosClient(inner client.Client, faults *FaultConfig) *ChaosClient` — Create a chaos-enabled client
- All standard `client.Client` methods (`Get`, `List`, `Create`, `Update`, `Delete`, `Patch`)
- Metadata methods (`Scheme()`, `RESTMapper()`) pass through without injection

### FaultConfig

Thread-safe configuration for fault injection behavior.

```go
type FaultConfig struct {
    Active bool                        // Enable/disable all faults
    Faults map[Operation]FaultSpec     // Per-operation fault specs
}

type FaultSpec struct {
    ErrorRate float64       // Probability of injecting an error (0.0-1.0)
    Error     string        // Error message to return
    Delay     time.Duration // Fixed delay before operation
    MaxDelay  time.Duration // Random delay ceiling (overrides Delay)
}
```

**Example: Dynamic Fault Activation**

```go
config := sdk.NewFaultConfig(faults)

// Temporarily disable faults
config.Deactivate()
// ... perform critical operations
config.Activate()

// Add a new fault at runtime (thread-safe)
config.SetFault(sdk.OpDelete, sdk.FaultSpec{
    ErrorRate: 0.05,
    Error:     "conflict: resource version mismatch",
})
```

### Supported Operations

| Operation | Constant | Description |
|-----------|----------|-------------|
| Get | `sdk.OpGet` | Retrieve a single object |
| List | `sdk.OpList` | Retrieve a list of objects |
| Create | `sdk.OpCreate` | Create a new object |
| Update | `sdk.OpUpdate` | Update an existing object |
| Delete | `sdk.OpDelete` | Delete an object |
| Patch | `sdk.OpPatch` | Apply a patch to an object |
| DeleteAllOf | `sdk.OpDeleteAllOf` | Delete multiple objects |
| Apply | `sdk.OpApply` | Apply a server-side apply configuration |

### ChaosError

Fault-injected errors are wrapped in `ChaosError`, allowing you to distinguish simulated failures from real API errors:

```go
import "errors"

if err != nil {
    var chaosErr *sdk.ChaosError
    if errors.As(err, &chaosErr) {
        // This is a chaos-injected error
        log.Info("chaos fault triggered", "op", chaosErr.Operation)
    } else {
        // This is a real API error
        return err
    }
}
```

## Fault Injection Primitives (`pkg/sdk/faults`)

[:octicons-link-external-16: View on pkg.go.dev](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-chaos/pkg/sdk/faults)

The `faults` package provides declarative fault injection for common failure modes. These are typically used in integration tests or fuzzing harnesses.

### Network Faults

```go
import "github.com/opendatahub-io/odh-platform-chaos/pkg/sdk/faults"

// Inject random latency
faults.NetworkLatency(50 * time.Millisecond, 200 * time.Millisecond)

// Drop packets
faults.PacketDrop(0.1)  // 10% packet loss
```

### Resource Faults

```go
// Simulate resource pressure
faults.CPUPressure(0.8)      // 80% CPU usage
faults.MemoryPressure(0.9)   // 90% memory usage
faults.DiskPressure(0.95)    // 95% disk usage
```

### Timing Faults

```go
// Inject timing issues
faults.ClockSkew(5 * time.Minute)       // System clock 5min ahead
faults.SlowDisk(100 * time.Millisecond) // I/O operations delayed
```

### Kubernetes-Specific Faults

```go
// Inject API server faults
faults.APIServerUnavailable()
faults.WatchConnectionDrop()
faults.InformerResync()
```

!!! tip "Integration with ChaosClient"
    These fault primitives can be combined with `ChaosClient` for comprehensive resilience testing. Use `faults.*` for system-level failures and `ChaosClient` for API-level failures.

## Fuzzing Harness (`pkg/sdk/fuzz`)

[:octicons-link-external-16: View on pkg.go.dev](https://pkg.go.dev/github.com/opendatahub-io/odh-platform-chaos/pkg/sdk/fuzz)

The `fuzz` package provides a property-based testing harness for operator reconciliation logic. It automatically generates random CRD mutations and verifies reconciliation invariants.

```go
import (
    "github.com/opendatahub-io/odh-platform-chaos/pkg/sdk/fuzz"
    "testing"
)

func TestMyOperatorFuzzing(t *testing.T) {
    harness := fuzz.NewHarness(fuzz.Config{
        Iterations: 1000,
        CRDType:    &myv1.MyResource{},
        Reconciler: myReconciler,
    })

    // Define invariants
    harness.AddInvariant("status-eventually-ready", func(obj runtime.Object) error {
        res := obj.(*myv1.MyResource)
        if res.Status.Conditions == nil {
            return errors.New("status conditions should not be nil")
        }
        return nil
    })

    if err := harness.Run(context.Background()); err != nil {
        t.Fatalf("fuzz test failed: %v", err)
    }
}
```

## Testing Utilities (`pkg/sdk`)

### ReconcilerWrapper

Wrap your reconciler to collect metrics and observe behavior:

```go
import "github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"

wrapped := sdk.WrapReconciler(myReconciler, sdk.ReconcilerMetrics{
    RecordDuration: true,
    RecordErrors:   true,
})
```

### ConfigMapLoader

Load chaos configurations from ConfigMaps for runtime control:

```go
config, err := sdk.LoadChaosConfigFromConfigMap(ctx, client, "chaos-config", "opendatahub")
if err != nil {
    // No chaos config found, use normal client
    config = nil
}
chaosClient := sdk.NewChaosClient(innerClient, config)
```

## Knowledge-Driven Generation (`pkg/model`)

The `model` package bridges operator knowledge models to the fuzz SDK. It provides runtime functions for building seed objects and invariants, plus code generation helpers for the CLI.

### SeedObjects

Creates typed K8s objects from an OperatorKnowledge model. Each `ManagedResource` maps to its correct Go type (Deployment, DaemonSet, ConfigMap, etc.), with GVK, name, namespace, and labels populated. Unknown kinds fall back to `Unstructured`.

```go
import "github.com/opendatahub-io/odh-platform-chaos/pkg/model"

k, _ := model.LoadKnowledge("knowledge/kserve.yaml")
seeds := model.SeedObjects(k)
// seeds contains typed client.Object values ready for fake client
```

### Invariants

Creates `fuzz.Invariant` functions from steady-state checks and Deployment replicas. Objects are `DeepCopy`'d to prevent shared mutable state between invariant closures.

```go
invariants := model.Invariants(k)
for _, inv := range invariants {
    h.AddInvariant(inv)
}
```

### SeedCorpusEntries

Encodes architectural traits (webhooks, finalizers, leader election, dependencies) as `(opMask, faultType, intensity)` tuples. These become `f.Add()` calls in generated fuzz tests, giving the fuzzer architecturally relevant starting points.

```go
entries := model.SeedCorpusEntries(k)
for _, e := range entries {
    f.Add(e.OpMask, e.FaultType, e.Intensity)
}
```

**Trait mapping:**

| Trait | Operations | Fault Type | Intensity |
|-------|-----------|------------|-----------|
| Base (always) | Get, List | connection | 30% |
| Webhooks | Create, Update | webhook denied | 50% |
| Finalizers | Delete | conflict | 50% |
| Leader Election (Lease) | Get, Update | timeout | 40% |
| Dependencies | Get, List | not-found | 60% |

### Code Generation Helpers

`SeedObjectCode(mr)` and `InvariantCode(kind, name, ns)` return Go source strings for template rendering. These are used by the CLI's `generate fuzz-targets` command.

## Best Practices

!!! warning "Production Safety"
    Never enable `ChaosClient` in production namespaces. Use namespace-based or environment-based guards:

    ```go
    var faults *sdk.FaultConfig
    if os.Getenv("CHAOS_ENABLED") == "true" {
        faults = loadChaosConfig()
    }
    client := sdk.NewChaosClient(innerClient, faults)
    ```

!!! tip "Gradual Rollout"
    Start with low error rates (1-5%) and gradually increase as you gain confidence in your operator's resilience.

!!! example "CI Integration"
    Run fuzz tests in CI with a time budget:

    ```bash
    go test -v -timeout=10m -fuzz=FuzzReconciler ./pkg/controller
    ```

## Example: Complete Integration

```go
package controller

import (
    "context"
    "os"
    "time"

    "github.com/opendatahub-io/odh-platform-chaos/pkg/sdk"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type MyReconciler struct {
    Client client.Client
}

func (r *MyReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    // Your reconciliation logic here
    return reconcile.Result{}, nil
}

func NewReconciler(mgr manager.Manager) *MyReconciler {
    innerClient := mgr.GetClient()

    // Load chaos config from ConfigMap if present
    chaosConfig, _ := sdk.LoadChaosConfigFromConfigMap(
        context.Background(),
        innerClient,
        "odh-chaos-config",
        "opendatahub",
    )

    // Wrap client with chaos capabilities
    chaosClient := sdk.NewChaosClient(innerClient, chaosConfig)

    return &MyReconciler{
        Client: chaosClient,
    }
}
```

## Next Steps

- [Failure Modes Reference](../failure-modes/index.md) — Learn about cluster-level chaos experiments
- [Architecture: Injection Engine](../architecture/injection-engine.md) — Understand how injections are implemented
- [Contributing: Development Setup](../contributing/development-setup.md) — Set up a local development environment

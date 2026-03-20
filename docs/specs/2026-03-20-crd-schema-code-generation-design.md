# Controller Mode — CRD Schema and Code Generation Design Spec

**Jira:** RHOAIENG-52015 (3 SP)
**Date:** 2026-03-20
**Status:** Draft

## Problem

The `ChaosExperiment` type in `api/v1alpha1/types.go` is a plain Go struct with a custom `Metadata` type, custom `Duration` type, and no Kubernetes runtime integration. It cannot be registered as a CRD, has no deepcopy methods, no scheme registration, and no OpenAPI validation schema. This blocks the Controller Mode feature chain (52015 → 54105 → 54106).

## Goals

- Convert `ChaosExperiment` to a proper Kubernetes CRD type with kubebuilder markers
- Generate deepcopy methods and CRD YAML manifests via controller-gen
- Add Kubernetes-standard conditions to the status subresource
- Add OpenAPI validation markers for enum types and required fields
- Maintain full backward compatibility with existing CLI, experiment YAML files, and knowledge models
- Follow Kubernetes conventions throughout

## Non-Goals

- Implementing the controller reconciler (RHOAIENG-54105)
- Adding a validating admission webhook (future iteration)
- CEL cross-field validation rules (future iteration)

## Architecture

### Approach: In-Place Migration

Replace the existing types in `api/v1alpha1/types.go` with CRD-compatible types. The existing types were designed as "CRD-ready" (comment on line 12). All field names and YAML structure are already compatible with Kubernetes conventions.

### Type Changes

#### ChaosExperiment (Root Type)

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=chaos;ce
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Verdict",type=string,JSONPath=`.status.verdict`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.injection.type`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.component`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ChaosExperiment struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   ChaosExperimentSpec   `json:"spec,omitempty"`
    Status ChaosExperimentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ChaosExperimentList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []ChaosExperiment `json:"items"`
}
```

**Removed:**
- Custom `Metadata` struct — replaced by embedded `metav1.ObjectMeta`
- Explicit `APIVersion` and `Kind` string fields — replaced by embedded `metav1.TypeMeta`

**JSON tag changes (Kubernetes conventions):**
- `Spec` tag changes from `json:"spec"` to `json:"spec,omitempty"` — no practical impact since experiment specs are never empty
- `metadata` tag changes from `json:"metadata"` to `json:"metadata,omitempty"` — the CLI `Validate` function still catches empty `metadata.name`, so validation behavior is preserved

#### Custom Duration Removal

Replace `v1alpha1.Duration` with `metav1.Duration` throughout. Both serialize as strings (e.g., `"60s"`) via `time.ParseDuration`. The custom `Duration` type and its JSON/YAML marshal methods are deleted.

**Access pattern change:**
- Constructor: `v1alpha1.Duration{Duration: 60 * time.Second}` → `metav1.Duration{Duration: 60 * time.Second}` (package prefix change only)
- Field access: `d.Duration` → `d.Duration` (same syntax, both resolve to `time.Duration`)

**Affected fields:**
- `HypothesisSpec.RecoveryTimeout`
- `InjectionSpec.TTL`
- `SteadyStateDef.Timeout`
- `model.RecoveryExpectations.ReconcileTimeout` (in `pkg/model/knowledge.go`)

#### time.Time → metav1.Time (CRD Status Fields Only)

Replace `time.Time` and `*time.Time` with `metav1.Time` and `*metav1.Time` in CRD status types only:

- `ChaosExperimentStatus.StartTime` → `*metav1.Time`
- `ChaosExperimentStatus.EndTime` → `*metav1.Time`
- `CheckResult.Timestamp` → `metav1.Time`
- `InjectionEvent.Timestamp` → `metav1.Time`

**Not migrated** (internal-only types, not CRD fields):
- `pkg/reporter/` types — keep `time.Time`
- `pkg/observer/` types — keep `time.Time`
- `pkg/evaluator/` types — keep `time.Time`

**Serialization change:** `time.Time` uses RFC3339Nano (nanosecond precision). `metav1.Time` uses RFC3339 (second precision). This only affects status fields which are runtime-generated, not user-authored YAML.

### Kubebuilder Validation Markers

#### Enum Types

```go
// +kubebuilder:validation:Enum=PodKill;NetworkPartition;CRDMutation;ConfigDrift;WebhookDisrupt;RBACRevoke;FinalizerBlock;ClientFault
type InjectionType string

// +kubebuilder:validation:Enum="";low;medium;high
type DangerLevel string

// +kubebuilder:validation:Enum=Pending;SteadyStatePre;Injecting;Observing;SteadyStatePost;Evaluating;Complete;Aborted
type ExperimentPhase string

// +kubebuilder:validation:Enum=Resilient;Degraded;Failed;Inconclusive
type Verdict string

// +kubebuilder:validation:Enum=conditionTrue;resourceExists
type CheckType string
```

Note: `DangerLevel` allows empty string (`""`) because an unset danger level is valid (defaults based on injection type).

#### Required Fields and Constraints

```go
type BlastRadiusSpec struct {
    // +kubebuilder:validation:Minimum=1
    MaxPodsAffected    int      `json:"maxPodsAffected"`
    // +kubebuilder:validation:MinItems=1
    AllowedNamespaces  []string `json:"allowedNamespaces"`
    // ...
}
```

Required struct fields are enforced by not using `omitempty` in their JSON tags (existing behavior). Additional markers:
- `TargetSpec.Operator` — required (no omitempty, already correct)
- `TargetSpec.Component` — required (no omitempty, already correct)
- `HypothesisSpec.Description` — required (no omitempty, already correct)

### Status Enhancements: Conditions

```go
type ChaosExperimentStatus struct {
    Phase           ExperimentPhase    `json:"phase,omitempty"`
    Verdict         Verdict            `json:"verdict,omitempty"`
    StartTime       *metav1.Time       `json:"startTime,omitempty"`
    EndTime         *metav1.Time       `json:"endTime,omitempty"`
    Conditions      []metav1.Condition `json:"conditions,omitempty"`
    SteadyStatePre  *CheckResult       `json:"steadyStatePre,omitempty"`
    SteadyStatePost *CheckResult       `json:"steadyStatePost,omitempty"`
    InjectionLog    []InjectionEvent   `json:"injectionLog,omitempty"`
}
```

**Condition types:**

| Condition | True When | Reason Examples |
|-----------|-----------|-----------------|
| `SteadyStateEstablished` | Pre-check passed | `BaselineVerified` / `BaselineFailed` |
| `FaultInjected` | Injection succeeded | `InjectionApplied` / `InjectionFailed` |
| `RecoveryObserved` | Recovery observation complete | `RecoveryComplete` / `RecoveryTimeout` |
| `Complete` | Experiment finished (any verdict) | `VerdictRendered` / `ExperimentAborted` |

Enables: `kubectl wait --for=condition=Complete chaosexperiment/my-test --timeout=10m`

### Scheme Registration

New file `api/v1alpha1/groupversion_info.go`:

```go
// +groupName=chaos.opendatahub.io
package v1alpha1

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
    GroupVersion  = schema.GroupVersion{Group: "chaos.opendatahub.io", Version: "v1alpha1"}
    SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
    AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
    SchemeBuilder.Register(&ChaosExperiment{}, &ChaosExperimentList{})
}
```

New file `api/v1alpha1/doc.go`:

```go
// +groupName=chaos.opendatahub.io
package v1alpha1
```

The `+groupName` marker in `doc.go` is required for controller-gen to use the correct API group name instead of the Go package name.

### Code Generation

**Makefile targets:**

```makefile
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: generate
generate:
	$(CONTROLLER_GEN) object paths=./api/...

.PHONY: manifests
manifests:
	$(CONTROLLER_GEN) crd paths=./api/... output:crd:dir=config/crd/bases
```

**Generated files:**
- `api/v1alpha1/zz_generated.deepcopy.go` — `DeepCopyObject()`, `DeepCopyInto()` for all types
- `config/crd/bases/chaos.opendatahub.io_chaosexperiments.yaml` — CRD manifest with OpenAPI v3 validation schema

### YAML Loading Compatibility

**CLI strict loader (`pkg/experiment/loader.go`):**

The loader uses `yaml.UnmarshalStrict()` which rejects YAML fields not present on the Go struct. After migration to `metav1.ObjectMeta`:

- **Parsing existing YAMLs: SAFE.** Strict mode rejects fields *present in YAML but not defined on the struct*. Since `name`, `namespace`, and `labels` are all real fields on `metav1.ObjectMeta`, existing YAML files parse without changes. Extra ObjectMeta fields (`creationTimestamp`, `uid`, etc.) are simply zero-valued — they're defined on the struct but absent from the YAML, which strict mode allows.

- **Marshaling output changes.** `metav1.ObjectMeta` emits `creationTimestamp: null` even when zero-valued (the upstream K8s type is not `omitempty` for this field). Round-trip tests that compare marshaled output will need updating.

- **Dual-path loading.** The CLI continues using the strict YAML loader for user-authored experiment files. The controller (future task 54105) will use controller-runtime's built-in deserialization from the API server, which handles the full ObjectMeta.

**`sigs.k8s.io/yaml` compatibility:** This library converts YAML to JSON first, then uses `json:` struct tags. `metav1.ObjectMeta` only has `json:` tags (no `yaml:` tags). The current custom `Metadata` struct's explicit `yaml:` tags are redundant — `sigs.k8s.io/yaml` never uses them.

### Consumer Migration

**Field access pattern change:** `exp.Metadata.Name` → `exp.Name` (ObjectMeta is embedded, fields are promoted).

**Affected files (~20 files):**

| Package | Files | Change |
|---------|-------|--------|
| `api/v1alpha1/` | `types.go`, `types_test.go` | Type definitions, Duration constructors |
| `pkg/experiment/` | `loader.go`, `loader_test.go` | `Metadata.Name` → `Name`, Duration type |
| `pkg/orchestrator/` | `lifecycle.go`, `lifecycle_test.go` | `Metadata.Name/Namespace` → `Name/Namespace` (~8 refs) |
| `pkg/reporter/` | `json.go` | `Metadata.Name/Namespace` → `Name/Namespace` |
| `pkg/model/` | `knowledge.go`, `validate.go`, `validate_test.go` | Duration type change |
| `pkg/safety/` | if referencing Metadata | Field access |
| `internal/cli/` | `suite.go` (~7 refs), `validate.go`, `init_test.go`, `run.go` | Field access, Duration type |
| `pkg/evaluator/` | `engine_test.go` | Duration constructors in tests |

**Not affected:**
- 40 experiment YAML files (structure already compatible)
- 7 knowledge YAML files (Duration wire format unchanged)
- `init.go` template (uses `fmt.Sprintf`, already matches K8s format)

### SteadyStateCheck.APIVersion Field

The `SteadyStateCheck` struct has an `APIVersion` field representing the API version of the *target resource* to check (e.g., `apps/v1`). This is **not** the CRD's own TypeMeta APIVersion — `SteadyStateCheck` is a nested struct, not the root type. No collision occurs, but this should be clearly documented in godoc to avoid confusion.

## File Changes Summary

| File | Change |
|------|--------|
| `api/v1alpha1/types.go` | REWRITE — CRD types with kubebuilder markers, metav1 embedding, remove Metadata/Duration |
| `api/v1alpha1/doc.go` | NEW — `+groupName` marker |
| `api/v1alpha1/groupversion_info.go` | NEW — GroupVersion, SchemeBuilder, AddToScheme, init() |
| `api/v1alpha1/zz_generated.deepcopy.go` | GENERATED — by controller-gen |
| `api/v1alpha1/types_test.go` | UPDATE — metav1 types, add round-trip/deepcopy/scheme/YAML-compat tests |
| `config/crd/bases/chaos.opendatahub.io_chaosexperiments.yaml` | GENERATED — CRD manifest |
| `Makefile` | ADD — generate and manifests targets |
| `pkg/experiment/loader.go` | UPDATE — Metadata → ObjectMeta field access |
| `pkg/experiment/loader_test.go` | UPDATE — field access, Duration constructors |
| `pkg/orchestrator/lifecycle.go` | UPDATE — Metadata → ObjectMeta, Duration/Time types |
| `pkg/orchestrator/lifecycle_test.go` | UPDATE — field access, Duration constructors |
| `pkg/reporter/json.go` | UPDATE — Metadata → ObjectMeta |
| `pkg/model/knowledge.go` | UPDATE — Duration type |
| `pkg/model/validate.go` | UPDATE — Duration type |
| `pkg/model/validate_test.go` | UPDATE — Duration constructors |
| `pkg/evaluator/engine_test.go` | UPDATE — Duration constructors |
| `internal/cli/suite.go` | UPDATE — field access (~7 refs) |
| `internal/cli/validate.go` | UPDATE — field access |
| `internal/cli/init_test.go` | UPDATE — field access |
| `pkg/safety/*.go` | NO CHANGE EXPECTED — verify during implementation |
| `tests/integration/experiment_test.go` | UPDATE — Metadata struct literals, Duration constructors |

## Testing Strategy

1. **CRD round-trip serialization** — Marshal `ChaosExperiment` to JSON/YAML and back, verify no data loss. Expect `creationTimestamp` in marshaled output.

2. **Deepcopy test** — Verify `DeepCopyObject()` produces an independent copy. Mutate the copy, confirm original unchanged.

3. **Scheme registration test** — Verify `ChaosExperiment` and `ChaosExperimentList` are registered, GVK resolves correctly to `chaos.opendatahub.io/v1alpha1/ChaosExperiment`.

4. **CRD manifest validation** — Verify `kubectl apply -f config/crd/bases/chaos.opendatahub.io_chaosexperiments.yaml` succeeds and `kubectl get chaosexperiment` returns an empty list.

5. **YAML compatibility test** — Load existing experiment YAML files from `experiments/` into the new CRD type via the strict loader. Verify all fields parse correctly. This confirms backward compatibility.

6. **Existing test regression** — All existing tests must continue passing after migration.

7. **Duration wire format test** — Verify `metav1.Duration` serializes identically to the old custom Duration for values like `"60s"`, `"300s"`, `"5m0s"`.

8. **Condition type constants test** — Verify condition type strings are defined and usable with `meta.SetStatusCondition`.

## Backward Compatibility

- All 40 experiment YAML files parse without changes
- All 7 knowledge YAML files (plus 3 in `testdata/knowledge/`) parse without changes (Duration wire format unchanged)
- CLI commands continue working with the same flags and behavior
- JSON report format is unchanged (reporter keeps `time.Time` internally)
- The `init` command template already matches Kubernetes metadata format

## Nice-to-Have (Adopted)

- **Extra print columns:** INJECTION-TYPE and TARGET for richer `kubectl get` output
- **Short names:** `kubectl get chaos` or `kubectl get ce` via `+kubebuilder:resource:shortName`
- **Storage version marker:** `+kubebuilder:storageversion` for future version migration readiness
- **SteadyStateCheck.APIVersion godoc:** Document that this field refers to the target resource's API version, not the CRD's own TypeMeta

## Nice-to-Have (Deferred)

- **CEL validation rules** (`+kubebuilder:validation:XValidation`) for cross-field constraints (e.g., high danger requires `allowDangerous: true`). Deferred to a future iteration.
- **Validating admission webhook** for `Parameters` map validation per injection type. Cannot be expressed in OpenAPI schema markers.

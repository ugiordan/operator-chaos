# CRD Schema Reference

Auto-generated from `api/v1alpha1/types.go`.

## ChaosExperiment

ChaosExperiment defines a chaos engineering experiment.

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Spec` | `ChaosExperimentSpec` | `spec` |  |
| `Status` | `ChaosExperimentStatus` | `status` |  |

## ChaosExperimentList

ChaosExperimentList contains a list of ChaosExperiment.

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Items` | `[]ChaosExperiment` | `items` |  |

## ChaosExperimentSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Target` | `TargetSpec` | `target` |  |
| `SteadyState` | `SteadyStateSpec` | `steadyState` |  |
| `Injection` | `InjectionSpec` | `injection` |  |
| `BlastRadius` | `BlastRadiusSpec` | `blastRadius` |  |
| `Hypothesis` | `HypothesisSpec` | `hypothesis` |  |

## TargetSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Operator` | `string` | `operator` | +kubebuilder:validation:MinLength=1 |
| `Component` | `string` | `component` | +kubebuilder:validation:MinLength=1 |
| `Resource` | `string` | `resource` |  |

## SteadyStateSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Checks` | `[]SteadyStateCheck` | `checks` | +listType=atomic |
| `Timeout` | `metav1.Duration` | `timeout` |  |

## SteadyStateCheck

SteadyStateCheck defines a single check for steady-state verification.
Note: APIVersion refers to the target resource's API version (e.g. "apps/v1"),
not the CRD's own TypeMeta API version.

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Type` | `CheckType` | `type` |  |
| `APIVersion` | `string` | `apiVersion` |  |
| `Kind` | `string` | `kind` |  |
| `Name` | `string` | `name` |  |
| `Namespace` | `string` | `namespace` |  |
| `ConditionType` | `string` | `conditionType` |  |

## InjectionSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Type` | `InjectionType` | `type` |  |
| `Parameters` | `map[string]string` | `parameters` |  |
| `Count` | `int32` | `count` | Count is the number of targets to affect. Defaults to 1. +kubebuilder:validation:Minimum=1 +kubebuilder:validation:Maximum=100 +kubebuilder:default=1 |
| `TTL` | `metav1.Duration` | `ttl` |  |
| `DangerLevel` | `DangerLevel` | `dangerLevel` |  |

## BlastRadiusSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `MaxPodsAffected` | `int32` | `maxPodsAffected` | +kubebuilder:validation:Minimum=1 |
| `AllowedNamespaces` | `[]string` | `allowedNamespaces` | +listType=set |
| `ForbiddenResources` | `[]string` | `forbiddenResources` | +listType=set |
| `AllowDangerous` | `bool` | `allowDangerous` |  |
| `DryRun` | `bool` | `dryRun` |  |

## HypothesisSpec

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Description` | `string` | `description` | +kubebuilder:validation:MinLength=1 |
| `RecoveryTimeout` | `metav1.Duration` | `recoveryTimeout` | +optional |

## ChaosExperimentStatus

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Phase` | `ExperimentPhase` | `phase` |  |
| `Verdict` | `Verdict` | `verdict` |  |
| `ObservedGeneration` | `int64` | `observedGeneration` | +kubebuilder:validation:Minimum=0 |
| `Message` | `string` | `message` | Message provides a human-readable description of the current status. +optional |
| `StartTime` | `*metav1.Time` | `startTime` |  |
| `EndTime` | `*metav1.Time` | `endTime` |  |
| `InjectionStartedAt` | `*metav1.Time` | `injectionStartedAt` |  |
| `SteadyStatePre` | `*CheckResult` | `steadyStatePre` |  |
| `SteadyStatePost` | `*CheckResult` | `steadyStatePost` |  |
| `InjectionLog` | `[]InjectionEvent` | `injectionLog` | +listType=atomic |
| `EvaluationResult` | `*EvaluationSummary` | `evaluationResult` |  |
| `CleanupError` | `string` | `cleanupError` |  |
| `Conditions` | `[]metav1.Condition` | `conditions` | +listType=map +listMapKey=type |

## EvaluationSummary

EvaluationSummary is the CRD-embeddable evaluation result.

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Verdict` | `Verdict` | `verdict` |  |
| `Confidence` | `string` | `confidence` |  |
| `RecoveryTime` | `string` | `recoveryTime` |  |
| `ReconcileCycles` | `int` | `reconcileCycles` |  |
| `Deviations` | `[]string` | `deviations` | +listType=atomic |

## CheckResult

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Passed` | `bool` | `passed` |  |
| `ChecksRun` | `int32` | `checksRun` |  |
| `ChecksPassed` | `int32` | `checksPassed` |  |
| `Details` | `[]CheckDetail` | `details` | +listType=atomic |
| `Timestamp` | `metav1.Time` | `timestamp` |  |

## CheckDetail

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Check` | `SteadyStateCheck` | `check` |  |
| `Passed` | `bool` | `passed` |  |
| `Value` | `string` | `value` |  |
| `Error` | `string` | `error` |  |

## InjectionEvent

| Field | Type | JSON | Description |
|-------|------|------|-------------|
| `Timestamp` | `metav1.Time` | `timestamp` |  |
| `Type` | `InjectionType` | `type` |  |
| `Target` | `string` | `target` |  |
| `Action` | `string` | `action` |  |
| `Details` | `map[string]string` | `details` |  |


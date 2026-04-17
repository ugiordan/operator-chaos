# CRDMutation

**Danger Level:** :material-shield-alert: Medium

Mutates a spec field on a custom resource instance to test reconciliation of CR state.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `apiVersion` | `string` | Yes | - | API version of the target resource (e.g., serving.kserve.io/v1beta1) |
| `kind` | `string` | Yes | - | Kind of the target resource (e.g., InferenceService) |
| `name` | `string` | Yes | - | Name of the target resource instance |
| `field` | `string` | Yes | - | JSON path to the spec field to mutate (e.g., spec.predictor.minReplicas) |
| `value` | `string` | Yes | - | New value (JSON-typed: "999" becomes int, "true" becomes bool, '"text"' becomes string) |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

## How It Works

CRDMutation uses Unstructured client to read the target custom resource, saves the original field value, and applies a merge patch with the new value. The original value is stored in a rollback annotation.

**API calls:**
1. `Get` the target resource as Unstructured
2. Store original field value in rollback annotation
3. `Patch` the resource with a merge patch containing the new value
4. On cleanup: read rollback annotation, `Patch` back to original value

**Value type detection:** The injected value string is parsed as JSON: `"999"` becomes integer 999, `"true"` becomes boolean true, `"\"text\""` becomes string "text". If JSON parsing fails, the raw string is used.

**Cleanup:** Restores the original value via merge patch. If the original value was absent (nil), the field is removed.

**Crash safety:** Rollback annotation persists on the resource. `Revert` can restore even after crash.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator detects the spec mutation through its watch/informer and reconciles the resource back to the desired state, or updates dependent resources to match the new spec.

**Contract violation indicators:**
- Operator does not detect the mutation (watch not set up for this field)
- Operator enters infinite reconciliation loop (mutating the field triggers another reconciliation)
- Dependent resources become inconsistent with the CR spec

**Collateral damage risks:**
- Medium. The mutation affects a single CR instance, but the operator may propagate changes to dependent resources
- Requires a test CR to exist (experiments targeting production CRs should use `allowDangerous: true`)

**Recovery expectations:**
- Recovery time: 5-60 seconds (depends on reconciliation interval and complexity)
- Reconcile cycles: 1-2
- What "recovered" means: CR spec matches desired state, dependent resources are consistent

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| odh-model-controller | odh-model-controller-crd-mutation | medium | InferenceService has no scalar top-level spec fields, so this experiment injects... |
| odh-model-controller | odh-model-controller-leader-lease-corrupt | high | Controller detects corrupted leader lease holderIdentity and re-elects leader wi... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

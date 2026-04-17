# FinalizerBlock

**Danger Level:** :material-shield-alert: Medium

Adds a stuck finalizer to a resource to test deletion handling and cleanup logic.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `apiVersion` | `string` | No | `v1` | API version of the target resource |
| `kind` | `string` | Yes | - | Kind of the target resource |
| `name` | `string` | Yes | - | Name of the target resource |
| `finalizer` | `string` | No | `chaos.opendatahub.io/block` | Finalizer string to add |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

## How It Works

FinalizerBlock adds a finalizer to the target resource using an Unstructured client. When the resource is subsequently deleted, it enters a `Terminating` state and cannot be fully removed until the finalizer is cleared.

**API calls:**
1. `Get` the target resource as Unstructured
2. Add the finalizer to `metadata.finalizers` list
3. `Update` the resource
4. On cleanup: `Get` the resource, remove the finalizer, clean chaos metadata, `Update`

**Cleanup:** Removes the added finalizer. If the resource is in Terminating state, this unblocks deletion.

**Crash safety:** The finalizer persists on the resource. `Revert` removes it. Use `odh-chaos clean` for orphaned finalizers.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator should detect the stuck finalizer and either: (a) handle it as part of normal cleanup logic, or (b) surface a clear status condition indicating the resource cannot be deleted. The operator should not hang or deadlock.

**Contract violation indicators:**
- Operator hangs waiting for resource deletion (indicates synchronous delete-and-wait pattern)
- Operator enters infinite loop trying to delete the resource (indicates missing finalizer handling)
- Other resources dependent on the stuck resource are orphaned

**Collateral damage risks:**
- Medium. Only the target resource is affected
- If the resource is a dependency for other resources, cascading effects are possible
- Requires a test resource instance (not a production resource)

**Recovery expectations:**
- Recovery time: immediate after finalizer removal
- Reconcile cycles: 1 (operator detects finalizer removal and completes deletion)
- What "recovered" means: resource is either fully deleted or back to normal state without the chaos finalizer

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| data-science-pipelines | data-science-pipelines-finalizer-block | low | When a stuck finalizer prevents a DataSciencePipelinesApplication from being del... |
| kueue | kueue-finalizer-block | low | When a stuck finalizer prevents a Workload from being deleted, the controller sh... |
| model-registry | model-registry-finalizer-block | low | When a stuck finalizer prevents a ModelRegistry from being deleted, the operator... |
| odh-model-controller | odh-model-controller-finalizer-block | low | When a stuck finalizer prevents an InferenceService from being deleted, the odh-... |
| opendatahub-operator | opendatahub-operator-finalizer-block | low | When a stuck finalizer prevents a DataScienceCluster from being deleted, the ope... |
| ray | ray-finalizer-block | low | When a stuck finalizer prevents a RayCluster from being deleted, the controller ... |
| training-operator | training-operator-finalizer-block | low | When a stuck finalizer prevents a PyTorchJob from being deleted, the controller ... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

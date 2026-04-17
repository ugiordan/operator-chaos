# RBACRevoke

**Danger Level:** :material-shield-remove: High

Clears all subjects from a ClusterRoleBinding or RoleBinding to test RBAC resilience.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bindingType` | `string` | Yes | - | Type of binding: ClusterRoleBinding or RoleBinding |
| `bindingName` | `string` | Yes | - | Name of the binding to modify |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

## How It Works

RBACRevoke reads the target binding, serializes the original subjects list, then clears all subjects from the binding. This effectively revokes all permissions granted by the binding.

**API calls:**
1. `Get` the ClusterRoleBinding or RoleBinding
2. Serialize original subjects to rollback annotation
3. `Update` the binding with empty subjects list
4. On cleanup: deserialize original subjects, `Update` binding to restore them

**Cleanup:** Restores the original subjects list from the rollback annotation. Idempotent.

**Crash safety:** Rollback annotation persists on the binding resource.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator's ServiceAccount loses its permissions. API calls from the controller start failing with 403 Forbidden. The operator should handle permission errors gracefully (log errors, retry with backoff). Once permissions are restored, the operator should resume normal operation without manual intervention.

**Contract violation indicators:**
- Operator crashes on permission errors instead of retrying (indicates missing error handling)
- Operator does not recover after permissions are restored (indicates cached credentials not refreshed)
- Operator silently stops reconciling without surfacing errors (indicates swallowed errors)
- The binding is not reconciled back by the parent operator (indicates missing RBAC reconciliation)

**Collateral damage risks:**
- **High for ClusterRoleBindings** (cluster-scoped, affects all namespaces)
- Low for namespace-scoped RoleBindings
- Other controllers sharing the same ServiceAccount are also affected
- Requires `dangerLevel: high` and `allowDangerous: true` for ClusterRoleBindings

**Recovery expectations:**
- Recovery time: 10-60 seconds (depends on operator reconciliation of RBAC resources)
- Reconcile cycles: 1-2 (detect missing permissions, restore binding, resume)
- What "recovered" means: binding has original subjects, operator is actively reconciling

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| codeflare | codeflare-rbac-revoke | high | When the codeflare-operator ClusterRoleBinding subjects are revoked, the operato... |
| dashboard | dashboard-rbac-revoke | high | When the odh-dashboard ClusterRoleBinding subjects are revoked, the dashboard sh... |
| data-science-pipelines | data-science-pipelines-rbac-revoke | high | When the DSPO ClusterRoleBinding subjects are revoked, the operator should lose ... |
| feast | feast-rbac-revoke | high | When the feast-operator ClusterRoleBinding subjects are revoked, the operator lo... |
| kueue | kueue-rbac-revoke | high | When the kueue ClusterRoleBinding subjects are revoked, the controller can no lo... |
| llamastack | llamastack-rbac-revoke | high | When the llamastack ClusterRoleBinding subjects are revoked, the controller can ... |
| model-registry | model-registry-rbac-revoke | high | When the model-registry-operator ClusterRoleBinding subjects are revoked, the op... |
| modelmesh | modelmesh-rbac-revoke | high | When the modelmesh ClusterRoleBinding subjects are revoked, the controller can n... |
| odh-model-controller | odh-model-controller-rbac-revoke | high | When the odh-model-controller ClusterRoleBinding subjects are revoked, the contr... |
| opendatahub-operator | opendatahub-operator-rbac-revoke | high | When the operator ClusterRoleBinding subjects are revoked, the controller should... |
| ray | ray-rbac-revoke | high | When the ray-operator ClusterRoleBinding subjects are revoked, the controller ca... |
| training-operator | training-operator-rbac-revoke | high | When the training-operator ClusterRoleBinding subjects are revoked, the controll... |
| trustyai | trustyai-rbac-revoke | high | When the trustyai-service-operator ClusterRoleBinding subjects are revoked, the ... |
| workbenches | workbenches-rbac-revoke | high | When the odh-notebook-controller ClusterRoleBinding subjects are revoked, the co... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

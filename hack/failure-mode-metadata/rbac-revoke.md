---
name: RBACRevoke
type: RBACRevoke
danger: High
description: Clears all subjects from a ClusterRoleBinding or RoleBinding to test RBAC resilience.
spec_fields:
  - name: bindingType
    type: string
    required: true
    description: "Type of binding: ClusterRoleBinding or RoleBinding"
  - name: bindingName
    type: string
    required: true
    description: Name of the binding to modify
  - name: ttl
    type: duration
    required: false
    default: "300s"
    description: Auto-cleanup duration
---

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

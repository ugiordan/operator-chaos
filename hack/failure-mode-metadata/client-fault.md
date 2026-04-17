---
name: ClientFault
type: ClientFault
danger: Low
description: Injects errors, latency, or throttling into operator API calls via SDK integration.
spec_fields:
  - name: configMapName
    type: string
    required: false
    default: "odh-chaos-config"
    description: Name of the ConfigMap to store fault configuration
  - name: faults
    type: JSON
    required: true
    description: "JSON object mapping operation names to fault rules"
  - name: ttl
    type: duration
    required: false
    default: "300s"
    description: Auto-cleanup duration
---

## How It Works

ClientFault creates or updates a ConfigMap with fault injection configuration. Operators using the `sdk.ChaosClient` wrapper read this ConfigMap and apply faults to their Kubernetes API calls. This is an in-process fault injection mechanism that requires operator integration with the chaos SDK.

**API calls:**
1. `Get` the target ConfigMap (may not exist)
2. If exists: store original data in rollback state, `Update` with fault config
3. If not exists: `Create` ConfigMap with fault config, mark as "created by chaos"
4. On cleanup: restore original data or `Delete` if created by chaos

**Fault configuration schema:**
```json
{
  "operationName": {
    "errorRate": 0.1,
    "error": "context deadline exceeded",
    "delay": "50ms",
    "maxDelay": "200ms"
  }
}
```

Supported operations: `get`, `list`, `create`, `update`, `delete`, `patch`, `deleteAllOf`, `apply`

**Cleanup:** Restores original ConfigMap data or deletes the ConfigMap if it was created by the injector.

**Crash safety:** If created, the ConfigMap persists. Operators continue reading fault config until it is cleaned up.

## Disruption Rubric

**Expected behavior on a healthy operator (using chaos SDK):**
The operator experiences injected errors/latency on API calls. It should handle these gracefully with retry logic, backoff, and appropriate error surfacing. Reconciliation may be slower but should eventually converge.

**Contract violation indicators:**
- Operator does not retry on transient errors (indicates missing retry logic)
- Operator does not surface errors in status conditions (indicates swallowed errors)
- Reconciliation diverges or produces incorrect state under API errors

**Collateral damage risks:**
- Low. Only operators using `sdk.ChaosClient` are affected
- The ConfigMap is namespace-scoped
- No effect on operators not integrated with the chaos SDK

**Recovery expectations:**
- Recovery time: immediate after ConfigMap cleanup (faults stop on next config read)
- Reconcile cycles: 1-3 (catch up on delayed operations)
- What "recovered" means: operator reconciling normally without injected faults

**Prerequisite:** The target operator must integrate with the chaos SDK (`sdk.ChaosClient`). Without SDK integration, this injection type has no effect.

---
name: NetworkPartition
type: NetworkPartition
danger: Medium
description: Creates a deny-all NetworkPolicy isolating pods matching a label selector from all ingress and egress traffic.
spec_fields:
  - name: labelSelector
    type: string
    required: true
    description: Equality-based label selector for target pods (set-based selectors not supported)
  - name: ttl
    type: duration
    required: false
    default: "300s"
    description: Auto-cleanup duration for the NetworkPolicy
---

## How It Works

NetworkPartition creates a Kubernetes NetworkPolicy that blocks all ingress and egress traffic for pods matching the label selector. The policy name is sanitized (truncated to 63 chars with a hash suffix for uniqueness) and labeled with `app.kubernetes.io/managed-by: odh-chaos` for cleanup.

**API calls:**
1. Parse `labelSelector` into `metav1.LabelSelector`
2. `Create` a NetworkPolicy with deny-all ingress and egress rules
3. On cleanup: `Delete` the NetworkPolicy by name

**Cleanup:** Deletes the created NetworkPolicy. Traffic resumes immediately after deletion (no pod restart needed).

**Crash safety:** If the chaos tool crashes, the NetworkPolicy persists. Use `odh-chaos clean` to find and remove orphaned policies by the `managed-by` label.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator's pods lose network connectivity. API server calls from the controller fail. Once the NetworkPolicy is removed, the controller reconnects and resumes reconciliation. The Deployment should return to Available within `recoveryTimeout`.

**Contract violation indicators:**
- Controller enters CrashLoopBackOff after network is restored (indicates no retry/backoff logic)
- Controller does not resume reconciliation after partition ends (indicates lost watch connections without reconnect)
- Data corruption or inconsistent state after recovery (indicates missing conflict resolution)

**Collateral damage risks:**
- **High.** NetworkPolicy affects ALL pods matching the selector, not just the controller
- If the selector matches data-plane pods (e.g., serving pods), user traffic is disrupted
- On resource-constrained clusters (< 4 CPU per node), recovery may be slow due to scheduling pressure
- The NetworkPolicy requires a CNI that enforces policies (Calico, Cilium). Without enforcement, this test is meaningless.

**Recovery expectations:**
- Recovery time: 30-120 seconds depending on watch reconnection and leader election
- Reconcile cycles: 1-3 (initial reconnect, catch-up reconciliation, steady state)
- What "recovered" means: Deployment has `Available=True`, controller is actively reconciling
- **Known failure scenario:** On 2-node clusters with high CPU utilization (>80%), NetworkPartition consistently produces Failed verdicts due to recovery timeout pressure

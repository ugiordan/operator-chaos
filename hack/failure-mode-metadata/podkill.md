---
name: PodKill
type: PodKill
danger: Low
description: Force-deletes pods matching a label selector with zero grace period.
spec_fields:
  - name: labelSelector
    type: string
    required: true
    description: Equality-based label selector to match target pods (e.g., app=my-controller)
  - name: count
    type: int
    required: false
    default: "1"
    description: Number of pods to kill
  - name: ttl
    type: duration
    required: false
    default: "300s"
    description: Auto-cleanup duration
---

## How It Works

PodKill uses the Kubernetes API to delete pods matching the specified label selector with a zero grace period (`GracePeriodSeconds: 0`). When multiple pods match, the injector shuffles the list and kills up to `count` pods.

**API calls:**
1. `List` pods matching `labelSelector` in the target namespace
2. `Delete` each selected pod with `GracePeriodSeconds: 0`

**Cleanup:** No cleanup needed. The owning controller (Deployment, StatefulSet, DaemonSet) automatically recreates deleted pods.

**Crash safety:** Fully crash-safe. If the chaos tool crashes mid-injection, the owning controller still recreates pods. No rollback annotations needed.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator's Deployment controller recreates the pod within seconds. The new pod passes readiness probes and the Deployment returns to `Available` condition within the `recoveryTimeout`. If the operator uses leader election, the new pod acquires the lease and resumes reconciliation.

**Contract violation indicators:**
- Pod is not recreated (missing owning controller or broken replica count)
- New pod enters CrashLoopBackOff (indicates a startup dependency issue)
- Deployment stays unavailable beyond `recoveryTimeout` (indicates slow readiness)
- Reconciliation does not resume after pod restart (indicates state loss)

**Collateral damage risks:**
- Minimal. PodKill only affects pods matching the exact label selector
- If the controller manages stateful resources (Leases, PVCs), verify they are not orphaned
- On resource-constrained clusters, the new pod may be slow to schedule

**Recovery expectations:**
- Recovery time: typically 10-30 seconds for a healthy Deployment
- Reconcile cycles: 1 (the Deployment controller's standard behavior)
- What "recovered" means: Deployment has `Available=True` condition

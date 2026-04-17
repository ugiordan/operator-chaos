# PodKill

**Danger Level:** :material-shield-check: Low

Force-deletes pods matching a label selector with zero grace period.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `labelSelector` | `string` | Yes | - | Equality-based label selector to match target pods (e.g., app=my-controller) |
| `count` | `int` | No | `1` | Number of pods to kill |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

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

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| codeflare | codeflare-pod-kill | low | When the codeflare-operator pod is killed, existing Ray clusters remain unaffect... |
| dashboard | dashboard-pod-kill | low | When one odh-dashboard pod is killed, the remaining replica should continue serv... |
| data-science-pipelines | data-science-pipelines-pod-kill | low | When the data-science-pipelines-operator pod is killed, Kubernetes should recrea... |
| feast | feast-pod-kill | low | When the feast-operator pod is killed, existing FeatureStore instances continue ... |
| kserve | kserve-main-controller-kill | low | When the kserve-controller-manager pod is killed, the Deployment controller recr... |
| kueue | kueue-pod-kill | low | When the kueue-controller-manager pod is killed, pending workloads should queue ... |
| llamastack | llamastack-pod-kill | low | When the llamastack-controller-manager pod is killed, existing LlamaStack distri... |
| model-registry | model-registry-pod-kill | low | When the model-registry-operator pod is killed, Kubernetes should recreate it wi... |
| modelmesh | modelmesh-pod-kill | low | When the modelmesh-controller pod is killed, existing model endpoints keep servi... |
| odh-model-controller | odh-model-controller-pod-kill | low | When the odh-model-controller pod is killed, Kubernetes should recreate it withi... |
| opendatahub-operator | opendatahub-operator-pod-kill | low | When one operator pod is killed, the remaining replicas should maintain the lead... |
| ray | ray-pod-kill | low | When the ray-operator pod is killed, existing RayClusters keep running and servi... |
| training-operator | training-operator-pod-kill | low | When the training-operator pod is killed, running training jobs continue via wor... |
| trustyai | trustyai-pod-kill | low | When the trustyai-service-operator pod is killed, existing TrustyAI services kee... |
| workbenches | workbenches-pod-kill | low | When the odh-notebook-controller pod is killed, Kubernetes should recreate it wi... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

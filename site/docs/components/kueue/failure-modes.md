# kueue Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a Workload from being deleted, the controller sh... |
| NetworkPartition | medium | network-partition.yaml | When kueue-controller-manager pods are network-partitioned from the API server, ... |
| PodKill | low | pod-kill.yaml | When the kueue-controller-manager pod is killed, pending workloads should queue ... |
| RBACRevoke | high | rbac-revoke.yaml | When the kueue ClusterRoleBinding subjects are revoked, the controller can no lo... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the kueue validating webhook failurePolicy is weakened from Fail to Ignore,... |

## Experiment Details

### kueue-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** kueue-controller-manager

When a stuck finalizer prevents a Workload from being deleted, the controller should handle the Terminating state gracefully and not leak associated resource quota reservations. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-finalizer-block
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
    resource: Workload
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    parameters:
      apiVersion: kueue.x-k8s.io/v1beta1
      kind: Workload
      name: test-workload
      finalizer: kueue.x-k8s.io/managed-resources
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a Workload from being deleted, the
      controller should handle the Terminating state gracefully and not
      leak associated resource quota reservations. The chaos framework
      removes the finalizer via TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### kueue-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** kueue-controller-manager

When kueue-controller-manager pods are network-partitioned from the API server, workload admission stops and no new workloads are scheduled. Existing admitted workloads continue running. Once the partition is removed, scheduling resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-network-partition
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
    resource: Deployment/kueue-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=kueue
    ttl: "300s"
  hypothesis:
    description: >-
      When kueue-controller-manager pods are network-partitioned from the
      API server, workload admission stops and no new workloads are
      scheduled. Existing admitted workloads continue running. Once the
      partition is removed, scheduling resumes without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### kueue-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** kueue-controller-manager

When the kueue-controller-manager pod is killed, pending workloads should queue but not be admitted during downtime. Kubernetes should recreate the pod, and the controller should recover and resume scheduling within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-pod-kill
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
    resource: Deployment/kueue-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=kueue
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the kueue-controller-manager pod is killed, pending workloads
      should queue but not be admitted during downtime. Kubernetes should
      recreate the pod, and the controller should recover and resume
      scheduling within the recovery timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### kueue-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** kueue-controller-manager

When the kueue ClusterRoleBinding subjects are revoked, the controller can no longer read or update Workloads, ClusterQueues, or LocalQueues. Admission stops with 403 errors. Once permissions are restored, normal scheduling resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-rbac-revoke
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
    resource: ClusterRoleBinding/kueue-controller-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: kueue-controller-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the kueue ClusterRoleBinding subjects are revoked, the controller
      can no longer read or update Workloads, ClusterQueues, or LocalQueues.
      Admission stops with 403 errors. Once permissions are restored, normal
      scheduling resumes without restart.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### kueue-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** kueue-controller-manager

When the kueue validating webhook failurePolicy is weakened from Fail to Ignore, invalid Workload and ClusterQueue specs can be submitted bypassing validation. The controller should handle invalid resources gracefully. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: kueue-webhook-disrupt
spec:
  target:
    operator: kueue
    component: kueue-controller-manager
    resource: ValidatingWebhookConfiguration/vworkload.kb.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kueue-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: vworkload.kb.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the kueue validating webhook failurePolicy is weakened from Fail
      to Ignore, invalid Workload and ClusterQueue specs can be submitted
      bypassing validation. The controller should handle invalid resources
      gracefully. The chaos framework restores the original failurePolicy
      via TTL-based cleanup after 60s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

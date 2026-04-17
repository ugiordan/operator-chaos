# training-operator Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a PyTorchJob from being deleted, the controller ... |
| NetworkPartition | medium | network-partition.yaml | When the training-operator is network-partitioned from the API server, job statu... |
| PodKill | low | pod-kill.yaml | When the training-operator pod is killed, running training jobs continue via wor... |
| RBACRevoke | high | rbac-revoke.yaml | When the training-operator ClusterRoleBinding subjects are revoked, the controll... |

## Experiment Details

### training-operator-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** training-operator-controller-manager

When a stuck finalizer prevents a PyTorchJob from being deleted, the controller should handle the Terminating state gracefully and not leak associated worker pods or services. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: training-operator-finalizer-block
spec:
  target:
    operator: training-operator
    component: training-operator-controller-manager
    resource: PyTorchJob
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: training-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    parameters:
      apiVersion: kubeflow.org/v1
      kind: PyTorchJob
      name: test-pytorchjob
      finalizer: training-operator.kubeflow.org/finalizer
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a PyTorchJob from being deleted, the
      controller should handle the Terminating state gracefully and not leak
      associated worker pods or services. The chaos framework removes the
      finalizer via TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### training-operator-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** training-operator-controller-manager

When the training-operator is network-partitioned from the API server, job status stops updating but running worker pods continue training. Once the partition is removed, reconciliation resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: training-operator-network-partition
spec:
  target:
    operator: training-operator
    component: training-operator-controller-manager
    resource: Deployment/training-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: training-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=training-operator
    ttl: "300s"
  hypothesis:
    description: >-
      When the training-operator is network-partitioned from the API server,
      job status stops updating but running worker pods continue training.
      Once the partition is removed, reconciliation resumes without manual
      intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### training-operator-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** training-operator-controller-manager

When the training-operator pod is killed, running training jobs continue via worker pods. New job submissions queue until the controller recovers within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: training-operator-pod-kill
spec:
  target:
    operator: training-operator
    component: training-operator-controller-manager
    resource: Deployment/training-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: training-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=training-operator
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the training-operator pod is killed, running training jobs
      continue via worker pods. New job submissions queue until the
      controller recovers within the recovery timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### training-operator-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** training-operator-controller-manager

When the training-operator ClusterRoleBinding subjects are revoked, the controller can no longer manage PyTorchJob resources. API calls return 403 errors. Once permissions are restored, normal operation resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: training-operator-rbac-revoke
spec:
  target:
    operator: training-operator
    component: training-operator-controller-manager
    resource: ClusterRoleBinding/training-operator-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: training-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: training-operator-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the training-operator ClusterRoleBinding subjects are revoked,
      the controller can no longer manage PyTorchJob resources. API calls
      return 403 errors. Once permissions are restored, normal operation
      resumes without restart.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

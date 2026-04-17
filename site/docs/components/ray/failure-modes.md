# ray Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a RayCluster from being deleted, the controller ... |
| NetworkPartition | medium | network-partition.yaml | When the ray-operator is network-partitioned from the API server, cluster scalin... |
| PodKill | low | pod-kill.yaml | When the ray-operator pod is killed, existing RayClusters keep running and servi... |
| RBACRevoke | high | rbac-revoke.yaml | When the ray-operator ClusterRoleBinding subjects are revoked, the controller ca... |

## Experiment Details

### ray-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** ray-operator-controller-manager

When a stuck finalizer prevents a RayCluster from being deleted, the controller should handle the Terminating state gracefully and not leak associated head or worker pods. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: ray-finalizer-block
spec:
  target:
    operator: ray
    component: ray-operator-controller-manager
    resource: RayCluster
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: ray-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    parameters:
      apiVersion: ray.io/v1
      kind: RayCluster
      name: test-raycluster
      finalizer: ray.io/finalizer
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a RayCluster from being deleted, the
      controller should handle the Terminating state gracefully and not leak
      associated head or worker pods. The chaos framework removes the
      finalizer via TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### ray-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** ray-operator-controller-manager

When the ray-operator is network-partitioned from the API server, cluster scaling and health monitoring stops. Existing RayClusters continue running workloads. Once the partition is removed, reconciliation resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: ray-network-partition
spec:
  target:
    operator: ray
    component: ray-operator-controller-manager
    resource: Deployment/ray-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: ray-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=kuberay-operator
    ttl: "300s"
  hypothesis:
    description: >-
      When the ray-operator is network-partitioned from the API server,
      cluster scaling and health monitoring stops. Existing RayClusters
      continue running workloads. Once the partition is removed,
      reconciliation resumes without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### ray-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** ray-operator-controller-manager

When the ray-operator pod is killed, existing RayClusters keep running and serving workloads. New cluster requests queue until the controller recovers within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: ray-pod-kill
spec:
  target:
    operator: ray
    component: ray-operator-controller-manager
    resource: Deployment/ray-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: ray-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=kuberay-operator
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the ray-operator pod is killed, existing RayClusters keep
      running and serving workloads. New cluster requests queue until
      the controller recovers within the recovery timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### ray-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** ray-operator-controller-manager

When the ray-operator ClusterRoleBinding subjects are revoked, the controller can no longer manage RayCluster resources. API calls return 403 errors. Once permissions are restored, normal operation resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: ray-rbac-revoke
spec:
  target:
    operator: ray
    component: ray-operator-controller-manager
    resource: ClusterRoleBinding/ray-operator-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: ray-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: ray-operator-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the ray-operator ClusterRoleBinding subjects are revoked, the
      controller can no longer manage RayCluster resources. API calls
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

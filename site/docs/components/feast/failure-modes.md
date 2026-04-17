# feast Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| NetworkPartition | medium | network-partition.yaml | When the feast-operator is network-partitioned from the API server, FeatureStore... |
| PodKill | low | pod-kill.yaml | When the feast-operator pod is killed, existing FeatureStore instances continue ... |
| RBACRevoke | high | rbac-revoke.yaml | When the feast-operator ClusterRoleBinding subjects are revoked, the operator lo... |

## Experiment Details

### feast-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** feast-operator-controller-manager

When the feast-operator is network-partitioned from the API server, FeatureStore reconciliation stops. Existing feature servers remain available and continue serving features. Once the partition is removed, reconciliation resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: feast-network-partition
spec:
  target:
    operator: feast
    component: feast-operator-controller-manager
    resource: Deployment/feast-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: feast-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=feast-operator
    ttl: "300s"
  hypothesis:
    description: >-
      When the feast-operator is network-partitioned from the API server,
      FeatureStore reconciliation stops. Existing feature servers remain
      available and continue serving features. Once the partition is
      removed, reconciliation resumes without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### feast-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** feast-operator-controller-manager

When the feast-operator pod is killed, existing FeatureStore instances continue serving features. New FeatureStore deployments queue until the operator recovers and processes the backlog within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: feast-pod-kill
spec:
  target:
    operator: feast
    component: feast-operator-controller-manager
    resource: Deployment/feast-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: feast-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=feast-operator
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the feast-operator pod is killed, existing FeatureStore instances
      continue serving features. New FeatureStore deployments queue until
      the operator recovers and processes the backlog within the recovery
      timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### feast-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** feast-operator-controller-manager

When the feast-operator ClusterRoleBinding subjects are revoked, the operator loses cluster access and can no longer manage FeatureStore resources. API calls return 403 errors. Once permissions are restored, normal operation resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: feast-rbac-revoke
spec:
  target:
    operator: feast
    component: feast-operator-controller-manager
    resource: ClusterRoleBinding/feast-operator-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: feast-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: feast-operator-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the feast-operator ClusterRoleBinding subjects are revoked, the
      operator loses cluster access and can no longer manage FeatureStore
      resources. API calls return 403 errors. Once permissions are restored,
      normal operation resumes without restart.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

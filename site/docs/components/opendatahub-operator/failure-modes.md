# opendatahub-operator Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a DataScienceCluster from being deleted, the ope... |
| NetworkPartition | medium | network-partition.yaml | When the operator pods are network-partitioned, the leader should lose its lease... |
| PodKill | low | pod-kill.yaml | When one operator pod is killed, the remaining replicas should maintain the lead... |
| RBACRevoke | high | rbac-revoke.yaml | When the operator ClusterRoleBinding subjects are revoked, the controller should... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the validating webhook failurePolicy is weakened from Fail to Ignore, inval... |

## Experiment Details

### opendatahub-operator-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** opendatahub-operator

When a stuck finalizer prevents a DataScienceCluster from being deleted, the operator should handle the Terminating state gracefully, report the blocked deletion in its status, and not leak component deployments across managed namespaces. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-finalizer-block
spec:
  tier: 3
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
    resource: DataScienceCluster
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    # IMPORTANT: "default-dsc" is a placeholder. Replace it with the name
    # of the actual DataScienceCluster resource deployed in the cluster
    # before running this experiment.
    parameters:
      apiVersion: datasciencecluster.opendatahub.io/v1
      kind: DataScienceCluster
      name: default-dsc
      finalizer: platform.opendatahub.io/finalizer
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a DataScienceCluster from being
      deleted, the operator should handle the Terminating state gracefully,
      report the blocked deletion in its status, and not leak component
      deployments across managed namespaces. The chaos framework removes
      the finalizer via TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 3
    allowedNamespaces:
      - opendatahub-operator-system
```

</details>

### opendatahub-operator-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** opendatahub-operator

When the operator pods are network-partitioned, the leader should lose its lease and stop reconciling DSCInitialization and DataScienceCluster resources. Once connectivity is restored, a new leader election should occur and reconciliation should resume.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-network-partition
spec:
  tier: 2
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
    resource: Deployment/opendatahub-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager
    ttl: "300s"
  hypothesis:
    description: >-
      When the operator pods are network-partitioned, the leader should
      lose its lease and stop reconciling DSCInitialization and
      DataScienceCluster resources. Once connectivity is restored, a new
      leader election should occur and reconciliation should resume.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 3
    allowedNamespaces:
      - opendatahub-operator-system
```

</details>

### opendatahub-operator-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** opendatahub-operator

When one operator pod is killed, the remaining replicas should maintain the leader lease. Kubernetes should recreate the killed pod within the recovery timeout. The 3-replica HA deployment ensures continuous reconciliation of DataScienceCluster resources.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-pod-kill
spec:
  tier: 1
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
    resource: Deployment/opendatahub-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When one operator pod is killed, the remaining replicas should
      maintain the leader lease. Kubernetes should recreate the killed pod
      within the recovery timeout. The 3-replica HA deployment ensures
      continuous reconciliation of DataScienceCluster resources.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub-operator-system
```

</details>

### opendatahub-operator-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** opendatahub-operator

When the operator ClusterRoleBinding subjects are revoked, the controller should lose its ability to manage component deployments across namespaces and surface permission-denied errors. Once permissions are restored, reconciliation should resume without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-rbac-revoke
spec:
  tier: 4
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
    resource: ClusterRoleBinding/opendatahub-operator-controller-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: opendatahub-operator-controller-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the operator ClusterRoleBinding subjects are revoked, the
      controller should lose its ability to manage component deployments
      across namespaces and surface permission-denied errors. Once
      permissions are restored, reconciliation should resume without
      manual intervention.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 3
    allowDangerous: true
```

</details>

### opendatahub-operator-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** opendatahub-operator

When the validating webhook failurePolicy is weakened from Fail to Ignore, invalid DataScienceCluster and DSCInitialization resources can bypass admission validation. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: opendatahub-operator-webhook-disrupt
spec:
  tier: 4
  target:
    operator: opendatahub-operator
    component: opendatahub-operator
    resource: ValidatingWebhookConfiguration/validating.datasciencecluster.opendatahub.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: opendatahub-operator-controller-manager
        namespace: opendatahub-operator-system
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: validating.datasciencecluster.opendatahub.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the validating webhook failurePolicy is weakened from Fail to
      Ignore, invalid DataScienceCluster and DSCInitialization resources
      can bypass admission validation. The chaos framework restores the
      original failurePolicy via TTL-based cleanup after 60s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 3
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

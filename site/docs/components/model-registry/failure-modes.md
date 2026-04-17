# model-registry Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a ModelRegistry from being deleted, the operator... |
| NetworkPartition | medium | network-partition.yaml | When the model-registry-operator pod is network-partitioned from the API server,... |
| PodKill | low | pod-kill.yaml | When the model-registry-operator pod is killed, Kubernetes should recreate it wi... |
| RBACRevoke | high | rbac-revoke.yaml | When the model-registry-operator ClusterRoleBinding subjects are revoked, the op... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the ModelRegistry validating webhook failurePolicy is weakened from Fail to... |

## Experiment Details

### model-registry-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** model-registry-operator

When a stuck finalizer prevents a ModelRegistry from being deleted, the operator should handle the Terminating state gracefully, report the blocked deletion in its status, and not leak associated database or service resources. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-finalizer-block
spec:
  target:
    operator: model-registry
    component: model-registry-operator
    resource: ModelRegistry
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: model-registry-operator-controller-manager
        namespace: odh-model-registries
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    # IMPORTANT: "test-registry" is a placeholder. Replace it with the name
    # of an actual ModelRegistry resource deployed in the target namespace
    # before running this experiment.
    parameters:
      apiVersion: modelregistry.opendatahub.io/v1alpha1
      kind: ModelRegistry
      name: test-registry
      finalizer: modelregistry.opendatahub.io/finalizer
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a ModelRegistry from being deleted,
      the operator should handle the Terminating state gracefully, report
      the blocked deletion in its status, and not leak associated database
      or service resources. The chaos framework removes the finalizer via
      TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
```

</details>

### model-registry-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** model-registry-operator

When the model-registry-operator pod is network-partitioned from the API server, it should lose its leader lease and stop reconciling. Once the partition is removed, the operator should re-acquire the lease and resume normal operation.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-network-partition
spec:
  target:
    operator: model-registry
    component: model-registry-operator
    resource: Deployment/model-registry-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: model-registry-operator-controller-manager
        namespace: odh-model-registries
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=model-registry-operator
    ttl: "300s"
  hypothesis:
    description: >-
      When the model-registry-operator pod is network-partitioned from
      the API server, it should lose its leader lease and stop reconciling.
      Once the partition is removed, the operator should re-acquire the
      lease and resume normal operation.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
```

</details>

### model-registry-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** model-registry-operator

When the model-registry-operator pod is killed, Kubernetes should recreate it within the recovery timeout. The operator should resume reconciling ModelRegistry resources without data loss or registry downtime.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-pod-kill
spec:
  target:
    operator: model-registry
    component: model-registry-operator
    resource: Deployment/model-registry-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: model-registry-operator-controller-manager
        namespace: odh-model-registries
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=model-registry-operator
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the model-registry-operator pod is killed, Kubernetes should
      recreate it within the recovery timeout. The operator should resume
      reconciling ModelRegistry resources without data loss or registry
      downtime.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
```

</details>

### model-registry-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** model-registry-operator

When the model-registry-operator ClusterRoleBinding subjects are revoked, the operator should lose its ability to manage ModelRegistry instances and surface permission-denied errors. Once permissions are restored, reconciliation should resume without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-rbac-revoke
spec:
  target:
    operator: model-registry
    component: model-registry-operator
    resource: ClusterRoleBinding/model-registry-operator-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: model-registry-operator-controller-manager
        namespace: odh-model-registries
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: model-registry-operator-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the model-registry-operator ClusterRoleBinding subjects are
      revoked, the operator should lose its ability to manage ModelRegistry
      instances and surface permission-denied errors. Once permissions are
      restored, reconciliation should resume without manual intervention.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### model-registry-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** model-registry-operator

When the ModelRegistry validating webhook failurePolicy is weakened from Fail to Ignore, invalid ModelRegistry resources can bypass admission validation. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-webhook-disrupt
spec:
  target:
    operator: model-registry
    component: model-registry-operator
    resource: ValidatingWebhookConfiguration/vmodelregistry.opendatahub.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: model-registry-operator-controller-manager
        namespace: odh-model-registries
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: vmodelregistry.opendatahub.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the ModelRegistry validating webhook failurePolicy is weakened
      from Fail to Ignore, invalid ModelRegistry resources can bypass
      admission validation. The chaos framework restores the original
      failurePolicy via TTL-based cleanup after 60s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

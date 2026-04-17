# workbenches Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| NetworkPartition | medium | network-partition.yaml | When the odh-notebook-controller pod is network-partitioned from the API server,... |
| PodKill | low | pod-kill.yaml | When the odh-notebook-controller pod is killed, Kubernetes should recreate it wi... |
| RBACRevoke | high | rbac-revoke.yaml | When the odh-notebook-controller ClusterRoleBinding subjects are revoked, the co... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the notebook mutating webhook failurePolicy is weakened from Fail to Ignore... |

## Experiment Details

### workbenches-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** odh-notebook-controller

When the odh-notebook-controller pod is network-partitioned from the API server, it should stop reconciling notebook resources. Running notebooks should continue operating independently. Once the partition is removed, the controller should resume normal operation.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: workbenches-network-partition
spec:
  target:
    operator: workbenches
    component: odh-notebook-controller
    resource: Deployment/odh-notebook-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-notebook-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: app=odh-notebook-controller
    ttl: "300s"
  hypothesis:
    description: >-
      When the odh-notebook-controller pod is network-partitioned from
      the API server, it should stop reconciling notebook resources.
      Running notebooks should continue operating independently. Once the
      partition is removed, the controller should resume normal operation.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### workbenches-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** odh-notebook-controller

When the odh-notebook-controller pod is killed, Kubernetes should recreate it within the recovery timeout. The controller should resume managing notebook workbenches without interrupting running notebook sessions.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: workbenches-pod-kill
spec:
  target:
    operator: workbenches
    component: odh-notebook-controller
    resource: Deployment/odh-notebook-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-notebook-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: app=odh-notebook-controller
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the odh-notebook-controller pod is killed, Kubernetes should
      recreate it within the recovery timeout. The controller should
      resume managing notebook workbenches without interrupting running
      notebook sessions.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### workbenches-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** odh-notebook-controller

When the odh-notebook-controller ClusterRoleBinding subjects are revoked, the controller should lose its ability to manage notebook resources across namespaces and surface permission-denied errors. Once permissions are restored, reconciliation should resume.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: workbenches-rbac-revoke
spec:
  target:
    operator: workbenches
    component: odh-notebook-controller
    resource: ClusterRoleBinding/odh-notebook-controller-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-notebook-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: odh-notebook-controller-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the odh-notebook-controller ClusterRoleBinding subjects are
      revoked, the controller should lose its ability to manage notebook
      resources across namespaces and surface permission-denied errors.
      Once permissions are restored, reconciliation should resume.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### workbenches-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** odh-notebook-controller

When the notebook mutating webhook failurePolicy is weakened from Fail to Ignore, new notebooks may be created without the required sidecar injection and OAuth proxy configuration. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: workbenches-webhook-disrupt
spec:
  target:
    operator: workbenches
    component: odh-notebook-controller
    resource: MutatingWebhookConfiguration/notebooks.opendatahub.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-notebook-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: notebooks.opendatahub.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the notebook mutating webhook failurePolicy is weakened from
      Fail to Ignore, new notebooks may be created without the required
      sidecar injection and OAuth proxy configuration. The chaos framework
      restores the original failurePolicy via TTL-based cleanup after 60s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

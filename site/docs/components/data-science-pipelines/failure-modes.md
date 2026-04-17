# data-science-pipelines Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a DataSciencePipelinesApplication from being del... |
| NetworkPartition | medium | network-partition.yaml | When the DSPO pod is network-partitioned from the API server, it should lose its... |
| PodKill | low | pod-kill.yaml | When the data-science-pipelines-operator pod is killed, Kubernetes should recrea... |
| RBACRevoke | high | rbac-revoke.yaml | When the DSPO ClusterRoleBinding subjects are revoked, the operator should lose ... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the pipeline version validating webhook failurePolicy is weakened from Fail... |

## Experiment Details

### data-science-pipelines-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** data-science-pipelines-operator

When a stuck finalizer prevents a DataSciencePipelinesApplication from being deleted, the DSPO should handle the Terminating state gracefully, report the blocked deletion in its status, and not leak associated pipeline resources. The chaos framework removes the finalizer via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: data-science-pipelines-finalizer-block
spec:
  target:
    operator: data-science-pipelines
    component: data-science-pipelines-operator
    resource: DataSciencePipelinesApplication
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: data-science-pipelines-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    # IMPORTANT: "test-dspa" is a placeholder. Replace it with the name of an
    # actual DataSciencePipelinesApplication resource deployed in the target
    # namespace before running this experiment.
    parameters:
      apiVersion: datasciencepipelinesapplications.opendatahub.io/v1alpha1
      kind: DataSciencePipelinesApplication
      name: test-dspa
      finalizer: datasciencepipelinesapplications.opendatahub.io/finalizer
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents a DataSciencePipelinesApplication
      from being deleted, the DSPO should handle the Terminating state
      gracefully, report the blocked deletion in its status, and not leak
      associated pipeline resources. The chaos framework removes the
      finalizer via TTL-based cleanup after 300s.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### data-science-pipelines-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** data-science-pipelines-operator

When the DSPO pod is network-partitioned from the API server, it should lose its leader lease and stop reconciling. Once the partition is removed, the operator should re-acquire the lease and resume normal operation without duplicate pipeline runs.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: data-science-pipelines-network-partition
spec:
  target:
    operator: data-science-pipelines
    component: data-science-pipelines-operator
    resource: Deployment/data-science-pipelines-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: data-science-pipelines-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: app.kubernetes.io/name=data-science-pipelines-operator
    ttl: "300s"
  hypothesis:
    description: >-
      When the DSPO pod is network-partitioned from the API server, it
      should lose its leader lease and stop reconciling. Once the partition
      is removed, the operator should re-acquire the lease and resume
      normal operation without duplicate pipeline runs.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### data-science-pipelines-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** data-science-pipelines-operator

When the data-science-pipelines-operator pod is killed, Kubernetes should recreate it within the recovery timeout. The operator should resume reconciling DataSciencePipelinesApplication resources without data loss or pipeline run interruption.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: data-science-pipelines-pod-kill
spec:
  target:
    operator: data-science-pipelines
    component: data-science-pipelines-operator
    resource: Deployment/data-science-pipelines-operator-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: data-science-pipelines-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: app.kubernetes.io/name=data-science-pipelines-operator
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the data-science-pipelines-operator pod is killed, Kubernetes
      should recreate it within the recovery timeout. The operator should
      resume reconciling DataSciencePipelinesApplication resources without
      data loss or pipeline run interruption.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### data-science-pipelines-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** data-science-pipelines-operator

When the DSPO ClusterRoleBinding subjects are revoked, the operator should lose its ability to reconcile DataSciencePipelinesApplication resources across namespaces and surface permission-denied errors. Once permissions are restored, reconciliation should resume.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: data-science-pipelines-rbac-revoke
spec:
  target:
    operator: data-science-pipelines
    component: data-science-pipelines-operator
    resource: ClusterRoleBinding/data-science-pipelines-operator-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: data-science-pipelines-operator-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: data-science-pipelines-operator-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the DSPO ClusterRoleBinding subjects are revoked, the operator
      should lose its ability to reconcile DataSciencePipelinesApplication
      resources across namespaces and surface permission-denied errors.
      Once permissions are restored, reconciliation should resume.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### data-science-pipelines-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** ds-pipelines-webhook

When the pipeline version validating webhook failurePolicy is weakened from Fail to Ignore, invalid pipeline versions can bypass admission validation. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: data-science-pipelines-webhook-disrupt
spec:
  target:
    operator: data-science-pipelines
    component: ds-pipelines-webhook
    resource: ValidatingWebhookConfiguration/validating.pipelineversions.pipelines.kubeflow.org
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: ds-pipelines-webhook
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: validating.pipelineversions.pipelines.kubeflow.org
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the pipeline version validating webhook failurePolicy is
      weakened from Fail to Ignore, invalid pipeline versions can bypass
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

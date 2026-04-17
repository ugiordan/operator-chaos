# modelmesh Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| ConfigDrift | high | config-drift.yaml | When the modelmesh serving configuration is corrupted, new model deployments rec... |
| NetworkPartition | medium | network-partition.yaml | When the modelmesh-controller is network-partitioned from the API server, model ... |
| PodKill | low | pod-kill.yaml | When the modelmesh-controller pod is killed, existing model endpoints keep servi... |
| RBACRevoke | high | rbac-revoke.yaml | When the modelmesh ClusterRoleBinding subjects are revoked, the controller can n... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the modelmesh ServingRuntime validating webhook failurePolicy is weakened f... |

## Experiment Details

### modelmesh-config-drift

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** modelmesh-controller

When the modelmesh serving configuration is corrupted, new model deployments receive wrong serving parameters. Existing deployments remain unaffected. The operator should detect the drift and reconcile the correct configuration.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: modelmesh-config-drift
spec:
  target:
    operator: modelmesh
    component: modelmesh-controller
    resource: ConfigMap/modelmesh-serving-config
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: modelmesh-serving-config
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: modelmesh-serving-config
      key: config.yaml
      value: '{"modelServing":{"grpcMaxMessageSize":"-1","restProxy":"invalid://broken"}}'
      resourceType: ConfigMap
    ttl: "300s"
  hypothesis:
    description: >-
      When the modelmesh serving configuration is corrupted, new model
      deployments receive wrong serving parameters. Existing deployments
      remain unaffected. The operator should detect the drift and reconcile
      the correct configuration.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### modelmesh-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** modelmesh-controller

When the modelmesh-controller is network-partitioned from the API server, model routing stops updating but existing routes continue working. Once the partition is removed, reconciliation resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: modelmesh-network-partition
spec:
  target:
    operator: modelmesh
    component: modelmesh-controller
    resource: Deployment/modelmesh-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: modelmesh-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=modelmesh-controller,app.kubernetes.io/name=modelmesh-controller
    ttl: "300s"
  hypothesis:
    description: >-
      When the modelmesh-controller is network-partitioned from the API
      server, model routing stops updating but existing routes continue
      working. Once the partition is removed, reconciliation resumes
      without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### modelmesh-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** modelmesh-controller

When the modelmesh-controller pod is killed, existing model endpoints keep serving via existing serving runtime pods. New ServingRuntime deployments queue until the controller recovers within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: modelmesh-pod-kill
spec:
  target:
    operator: modelmesh
    component: modelmesh-controller
    resource: Deployment/modelmesh-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: modelmesh-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=modelmesh-controller,app.kubernetes.io/name=modelmesh-controller
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the modelmesh-controller pod is killed, existing model endpoints
      keep serving via existing serving runtime pods. New ServingRuntime
      deployments queue until the controller recovers within the recovery
      timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### modelmesh-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** modelmesh-controller

When the modelmesh ClusterRoleBinding subjects are revoked, the controller can no longer manage ServingRuntimes. API calls return 403 errors. Once permissions are restored, normal operation resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: modelmesh-rbac-revoke
spec:
  target:
    operator: modelmesh
    component: modelmesh-controller
    resource: ClusterRoleBinding/modelmesh-controller-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: modelmesh-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: modelmesh-controller-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the modelmesh ClusterRoleBinding subjects are revoked, the
      controller can no longer manage ServingRuntimes. API calls return
      403 errors. Once permissions are restored, normal operation resumes
      without restart.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### modelmesh-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** modelmesh-controller

When the modelmesh ServingRuntime validating webhook failurePolicy is weakened from Fail to Ignore, invalid ServingRuntime specs can be submitted bypassing validation. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: modelmesh-webhook-disrupt
spec:
  target:
    operator: modelmesh
    component: modelmesh-controller
    resource: ValidatingWebhookConfiguration/vservingruntime.modelmesh.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: modelmesh-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: vservingruntime.modelmesh.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the modelmesh ServingRuntime validating webhook failurePolicy is
      weakened from Fail to Ignore, invalid ServingRuntime specs can be
      submitted bypassing validation. The chaos framework restores the
      original failurePolicy via TTL-based cleanup after 60s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

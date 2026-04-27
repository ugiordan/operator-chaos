# llamastack Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| ConfigDrift | high | config-drift.yaml | When the llamastack serving configuration is corrupted, new LLM deployments rece... |
| NetworkPartition | medium | network-partition.yaml | When the llamastack-controller-manager is network-partitioned from the API serve... |
| PodKill | low | pod-kill.yaml | When the llamastack-controller-manager pod is killed, existing LlamaStack distri... |
| RBACRevoke | high | rbac-revoke.yaml | When the llamastack ClusterRoleBinding subjects are revoked, the controller can ... |

## Experiment Details

### llamastack-config-drift

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** llamastack-controller-manager

When the llamastack serving configuration is corrupted, new LLM deployments receive invalid config and fail to start. Existing deployments remain unaffected. The operator should detect the drift and reconcile the correct configuration.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: llamastack-config-drift
spec:
  tier: 2
  target:
    operator: llamastack
    component: llamastack-controller-manager
    resource: ConfigMap/llamastack-serving-config
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: llamastack-serving-config
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: llamastack-serving-config
      key: config.yaml
      value: '{"serving":{"endpoint":"invalid://broken","timeout":"-1"}}'
      resourceType: ConfigMap
    ttl: "300s"
  hypothesis:
    description: >-
      When the llamastack serving configuration is corrupted, new LLM
      deployments receive invalid config and fail to start. Existing
      deployments remain unaffected. The operator should detect the drift
      and reconcile the correct configuration.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### llamastack-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** llamastack-controller-manager

When the llamastack-controller-manager is network-partitioned from the API server, reconciliation stops. Running LLM endpoints remain unaffected. Once the partition is removed, reconciliation resumes without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: llamastack-network-partition
spec:
  tier: 2
  target:
    operator: llamastack
    component: llamastack-controller-manager
    resource: Deployment/llamastack-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: llamastack-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=llamastack
    ttl: "300s"
  hypothesis:
    description: >-
      When the llamastack-controller-manager is network-partitioned from
      the API server, reconciliation stops. Running LLM endpoints remain
      unaffected. Once the partition is removed, reconciliation resumes
      without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### llamastack-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** llamastack-controller-manager

When the llamastack-controller-manager pod is killed, existing LlamaStack distributions continue serving LLM endpoints. New deployments queue until the controller recovers within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: llamastack-pod-kill
spec:
  tier: 1
  target:
    operator: llamastack
    component: llamastack-controller-manager
    resource: Deployment/llamastack-controller-manager
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: llamastack-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=controller-manager,app.kubernetes.io/name=llamastack
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the llamastack-controller-manager pod is killed, existing
      LlamaStack distributions continue serving LLM endpoints. New
      deployments queue until the controller recovers within the recovery
      timeout.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### llamastack-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** llamastack-controller-manager

When the llamastack ClusterRoleBinding subjects are revoked, the controller can no longer manage LlamaStack distributions. API calls return 403 errors. Once permissions are restored, normal operation resumes without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: llamastack-rbac-revoke
spec:
  tier: 4
  target:
    operator: llamastack
    component: llamastack-controller-manager
    resource: ClusterRoleBinding/llamastack-controller-manager-rolebinding
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: llamastack-controller-manager
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: llamastack-controller-manager-rolebinding
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the llamastack ClusterRoleBinding subjects are revoked, the
      controller can no longer manage LlamaStack distributions. API calls
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

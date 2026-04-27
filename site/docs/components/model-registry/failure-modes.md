# model-registry Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents a ModelRegistry from being deleted, the operator... |
| NetworkPartition | medium | network-partition.yaml | When the model-registry-operator pod is network-partitioned from the API server,... |
| PodKill | low | pod-kill.yaml | When the model-registry-operator pod is killed, Kubernetes should recreate it wi... |
| RBACRevoke | high | rbac-revoke.yaml | When the model-registry-operator ClusterRoleBinding subjects are revoked, the op... |
| CRDMutation | high | route-backend-disruption.yaml | Changing the model-registry Route backend service to a non-existent service simu... |
| CRDMutation | high | route-host-collision.yaml | Mutating the model-registry REST API Route host simulates a host collision or DN... |
| CRDMutation | high | route-tls-mutation.yaml | Changing the TLS termination mode on the model-registry REST API Route from edge... |
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
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-finalizer-block
spec:
  tier: 3
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
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-network-partition
spec:
  tier: 2
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
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-pod-kill
spec:
  tier: 1
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
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-rbac-revoke
spec:
  tier: 4
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

### model-registry-route-backend-disruption

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** model-registry-operator

Changing the model-registry Route backend service to a non-existent service simulates backend disruption. All API requests return 503. The operator should detect the broken backend reference and reconcile the Route to point to the correct service. Expected verdict: Resilient if the operator restores the backend, Vulnerable if the REST API continues to fail.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-route-backend-disruption
spec:
  tier: 3
  target:
    operator: model-registry
    component: model-registry-operator
    resource: Route/model-registry-operator-rest
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
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "model-registry-operator-rest"
      path: "spec.to.name"
      value: "chaos-nonexistent-service"
    ttl: "300s"
  hypothesis:
    description: >-
      Changing the model-registry Route backend service to a non-existent
      service simulates backend disruption. All API requests return 503.
      The operator should detect the broken backend reference and reconcile
      the Route to point to the correct service. Expected verdict:
      Resilient if the operator restores the backend, Vulnerable if the
      REST API continues to fail.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
    allowDangerous: true
```

</details>

### model-registry-route-host-collision

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** model-registry-operator

Mutating the model-registry REST API Route host simulates a host collision or DNS misconfiguration. The model-registry operator should detect the Route drift and reconcile the host back to its correct value. Expected verdict: Resilient if the operator restores the Route host, Vulnerable if the Route remains misconfigured and the REST API becomes unreachable.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-route-host-collision
spec:
  tier: 3
  target:
    operator: model-registry
    component: model-registry-operator
    resource: Route/model-registry-operator-rest
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
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "model-registry-operator-rest"
      path: "spec.host"
      value: "chaos-collision.apps.cluster.invalid"
    ttl: "300s"
  hypothesis:
    description: >-
      Mutating the model-registry REST API Route host simulates a
      host collision or DNS misconfiguration. The model-registry operator
      should detect the Route drift and reconcile the host back to its
      correct value. Expected verdict: Resilient if the operator restores
      the Route host, Vulnerable if the Route remains misconfigured and
      the REST API becomes unreachable.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
    allowDangerous: true
```

</details>

### model-registry-route-tls-mutation

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** model-registry-operator

Changing the TLS termination mode on the model-registry REST API Route from edge/reencrypt to passthrough breaks HTTPS access to the model-registry API. The operator should detect the TLS config drift and restore the correct termination mode. Expected verdict: Resilient if the operator reconciles the TLS settings, Vulnerable if the REST API becomes unreachable over HTTPS.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-route-tls-mutation
spec:
  tier: 3
  target:
    operator: model-registry
    component: model-registry-operator
    resource: Route/model-registry-operator-rest
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
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "model-registry-operator-rest"
      path: "spec.tls.termination"
      value: "passthrough"
    ttl: "300s"
  hypothesis:
    description: >-
      Changing the TLS termination mode on the model-registry REST API
      Route from edge/reencrypt to passthrough breaks HTTPS access to
      the model-registry API. The operator should detect the TLS config
      drift and restore the correct termination mode. Expected verdict:
      Resilient if the operator reconciles the TLS settings, Vulnerable
      if the REST API becomes unreachable over HTTPS.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - odh-model-registries
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
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: model-registry-webhook-disrupt
spec:
  tier: 4
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

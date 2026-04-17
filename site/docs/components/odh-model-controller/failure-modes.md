# odh-model-controller Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| ConfigDrift | high | config-drift.yaml | When the inferenceservice-config ConfigMap is corrupted with an invalid deployme... |
| CRDMutation | medium | crd-mutation.yaml | InferenceService has no scalar top-level spec fields, so this experiment injects... |
| FinalizerBlock | low | finalizer-block.yaml | When a stuck finalizer prevents an InferenceService from being deleted, the odh-... |
| ConfigDrift | high | ingress-config-corruption.yaml | When the ingress key in inferenceservice-config is emptied, the odh-model-contro... |
| CRDMutation | high | leader-lease-corrupt.yaml | Controller detects corrupted leader lease holderIdentity and re-elects leader wi... |
| NetworkPartition | medium | network-partition.yaml | When the odh-model-controller pod is network-partitioned from the API server, it... |
| PodKill | low | pod-kill.yaml | When the odh-model-controller pod is killed, Kubernetes should recreate it withi... |
| RBACRevoke | high | rbac-revoke.yaml | When the odh-model-controller ClusterRoleBinding subjects are revoked, the contr... |
| ClientFault | low | sdk-api-throttle.yaml | When 30% of Get and 20% of List operations are throttled with 500ms-1s delays, t... |
| ClientFault | high | sdk-conflict-storm.yaml | When 70% of Update and 50% of Patch operations fail with conflict errors, the co... |
| ClientFault | low | sdk-watch-disconnect.yaml | When 40% of reconcile operations encounter watch channel closures, the controlle... |
| ConfigDrift | high | webhook-cert-corrupt.yaml | All 7 webhooks fail after TLS cert corruption; cert-manager or operator restores... |
| WebhookDisrupt | high | webhook-disrupt.yaml | When the validating webhook failurePolicy is weakened from Fail to Ignore, inval... |

## Experiment Details

### odh-model-controller-config-drift

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** odh-model-controller

When the inferenceservice-config ConfigMap is corrupted with an invalid deployment mode, the odh-model-controller should detect the misconfiguration and either fall back to defaults or surface clear error conditions rather than silently failing.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-config-drift
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: ConfigMap/inferenceservice-config
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: inferenceservice-config
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: inferenceservice-config
      key: deploy
      value: '{"defaultDeploymentMode":"INVALID_MODE"}'
      resourceType: ConfigMap
    ttl: "300s"
  hypothesis:
    description: >-
      When the inferenceservice-config ConfigMap is corrupted with an
      invalid deployment mode, the odh-model-controller should detect
      the misconfiguration and either fall back to defaults or surface
      clear error conditions rather than silently failing.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### odh-model-controller-crd-mutation

- **Type:** CRDMutation
- **Danger Level:** medium
- **Component:** odh-model-controller

InferenceService has no scalar top-level spec fields, so this experiment injects an unknown field ("chaosTest") via merge patch. The controller should reconcile without error and not propagate the unknown field to downstream resources. The expected verdict is Resilient — the controller ignores unknown fields gracefully. The chaos framework removes the injected field via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-crd-mutation
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: InferenceService
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    # NOTE: InferenceService has no scalar top-level spec fields that can be
    # trivially patched. Instead we inject an unknown field ("chaosTest") into
    # spec to trigger reconciliation. The controller should treat the unknown
    # field gracefully and not propagate it to downstream resources.
    #
    # IMPORTANT: Replace "test-isvc" with the name of an actual InferenceService
    # resource deployed in the target namespace before running this experiment.
    parameters:
      apiVersion: "serving.kserve.io/v1beta1"
      kind: "InferenceService"
      name: "test-isvc"
      field: "chaosTest"
      value: "injected"
    ttl: "300s"
  hypothesis:
    description: >-
      InferenceService has no scalar top-level spec fields, so this
      experiment injects an unknown field ("chaosTest") via merge patch.
      The controller should reconcile without error and not propagate
      the unknown field to downstream resources. The expected verdict is
      Resilient — the controller ignores unknown fields gracefully.
      The chaos framework removes the injected field via TTL-based cleanup
      after 300s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-finalizer-block

- **Type:** FinalizerBlock
- **Danger Level:** low
- **Component:** odh-model-controller

When a stuck finalizer prevents an InferenceService from being deleted, the odh-model-controller should handle the Terminating state gracefully, report the blocked deletion in its status, and not leak associated resources such as Routes or Services.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-finalizer-block
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: InferenceService
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: FinalizerBlock
    # IMPORTANT: "test-isvc" is a placeholder. Replace it with the name of an
    # actual InferenceService resource deployed in the target namespace before
    # running this experiment. The experiment targets a specific CR instance,
    # so a real resource name is required.
    parameters:
      apiVersion: serving.kserve.io/v1beta1
      kind: InferenceService
      name: test-isvc
      finalizer: odh.inferenceservice.finalizers
    ttl: "300s"
  hypothesis:
    description: >-
      When a stuck finalizer prevents an InferenceService from being
      deleted, the odh-model-controller should handle the Terminating
      state gracefully, report the blocked deletion in its status, and
      not leak associated resources such as Routes or Services.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-ingress-config-corruption

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** odh-model-controller

When the ingress key in inferenceservice-config is emptied, the odh-model-controller should detect the invalid configuration and surface error conditions rather than silently failing. The ConfigMap is not owned by this controller, so recovery depends on the DSCI/DSC operator or manual restoration. The chaos framework restores the original value via TTL-based cleanup after 300s.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-ingress-config-corruption
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: ConfigMap/inferenceservice-config
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: inferenceservice-config
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: "inferenceservice-config"
      key: "ingress"
      value: "{}"
      resourceType: "ConfigMap"
    ttl: "300s"
  hypothesis:
    description: >-
      When the ingress key in inferenceservice-config is emptied, the
      odh-model-controller should detect the invalid configuration and
      surface error conditions rather than silently failing. The ConfigMap
      is not owned by this controller, so recovery depends on the
      DSCI/DSC operator or manual restoration. The chaos framework
      restores the original value via TTL-based cleanup after 300s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### odh-model-controller-leader-lease-corrupt

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-model-controller

Controller detects corrupted leader lease holderIdentity and re-elects leader within 60s, resuming reconciliation. CLEANUP RISK: The TTL-based cleanup restores the original holderIdentity value, which may overwrite a legitimately re-elected leader and cause a brief second disruption. The controller should recover from this via a second re-election.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-leader-lease-corrupt
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Lease/odh-model-controller.opendatahub.io
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: coordination.k8s.io/v1
        kind: Lease
        name: odh-model-controller.opendatahub.io
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "coordination.k8s.io/v1"
      kind: "Lease"
      name: "odh-model-controller.opendatahub.io"
      field: "holderIdentity"
      value: "chaos-injected-invalid"
    ttl: "120s"
  hypothesis:
    description: >-
      Controller detects corrupted leader lease holderIdentity and
      re-elects leader within 60s, resuming reconciliation.
      CLEANUP RISK: The TTL-based cleanup restores the original
      holderIdentity value, which may overwrite a legitimately
      re-elected leader and cause a brief second disruption. The
      controller should recover from this via a second re-election.
    recoveryTimeout: 60s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### odh-model-controller-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** odh-model-controller

When the odh-model-controller pod is network-partitioned from the API server, it should lose its leader lease and stop reconciling. Once the partition is removed, the controller should re-acquire the lease and resume normal operation without duplicate work.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-network-partition
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Deployment/odh-model-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: control-plane=odh-model-controller
    ttl: "300s"
  hypothesis:
    description: >-
      When the odh-model-controller pod is network-partitioned from the
      API server, it should lose its leader lease and stop reconciling.
      Once the partition is removed, the controller should re-acquire the
      lease and resume normal operation without duplicate work.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** odh-model-controller

When the odh-model-controller pod is killed, Kubernetes should recreate it within the recovery timeout and the controller should resume reconciling InferenceService resources without data loss.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-pod-kill
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Deployment/odh-model-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: control-plane=odh-model-controller
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When the odh-model-controller pod is killed, Kubernetes should
      recreate it within the recovery timeout and the controller should
      resume reconciling InferenceService resources without data loss.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** odh-model-controller

When the odh-model-controller ClusterRoleBinding subjects are revoked, the controller should lose its ability to reconcile cluster-scoped resources and surface permission-denied errors in its logs. Once permissions are restored, normal reconciliation should resume without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-rbac-revoke
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: ClusterRoleBinding/odh-model-controller-rolebinding-opendatahub
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: odh-model-controller-rolebinding-opendatahub
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the odh-model-controller ClusterRoleBinding subjects are
      revoked, the controller should lose its ability to reconcile
      cluster-scoped resources and surface permission-denied errors in
      its logs. Once permissions are restored, normal reconciliation
      should resume without manual intervention.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>

### odh-model-controller-sdk-api-throttle

- **Type:** ClientFault
- **Danger Level:** low
- **Component:** odh-model-controller

When 30% of Get and 20% of List operations are throttled with 500ms-1s delays, the controller should retry with backoff and eventually converge. InferenceService status should recover within the recovery timeout.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-sdk-api-throttle
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Deployment/odh-model-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: ClientFault
    parameters:
      faults: '{"get":{"errorRate":0.3,"error":"api server throttled","delay":"500ms"},"list":{"errorRate":0.2,"error":"api server throttled","delay":"1s"}}'
    ttl: "120s"
  hypothesis:
    description: >-
      When 30% of Get and 20% of List operations are throttled with
      500ms-1s delays, the controller should retry with backoff and
      eventually converge. InferenceService status should recover
      within the recovery timeout.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-sdk-conflict-storm

- **Type:** ClientFault
- **Danger Level:** high
- **Component:** odh-model-controller

When 70% of Update and 50% of Patch operations fail with conflict errors, the controller should handle optimistic concurrency failures gracefully, re-read the resource, and retry. The controller must not enter an infinite retry loop or leak goroutines.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-sdk-conflict-storm
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Deployment/odh-model-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: ClientFault
    dangerLevel: high
    parameters:
      faults: '{"update":{"errorRate":0.7,"error":"conflict: the object has been modified"},"patch":{"errorRate":0.5,"error":"conflict: the object has been modified"}}'
    ttl: "120s"
  hypothesis:
    description: >-
      When 70% of Update and 50% of Patch operations fail with conflict
      errors, the controller should handle optimistic concurrency failures
      gracefully, re-read the resource, and retry. The controller must not
      enter an infinite retry loop or leak goroutines.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### odh-model-controller-sdk-watch-disconnect

- **Type:** ClientFault
- **Danger Level:** low
- **Component:** odh-model-controller

When 40% of reconcile operations encounter watch channel closures, the controller-runtime informer should re-establish the watch and the controller should resume processing events. No resources should be orphaned during the disruption window.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-sdk-watch-disconnect
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Deployment/odh-model-controller
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: ClientFault
    parameters:
      faults: '{"reconcile":{"errorRate":0.4,"error":"watch channel closed"}}'
    ttl: "120s"
  hypothesis:
    description: >-
      When 40% of reconcile operations encounter watch channel closures,
      the controller-runtime informer should re-establish the watch and
      the controller should resume processing events. No resources should
      be orphaned during the disruption window.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### odh-model-controller-webhook-cert-corrupt

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** odh-model-controller

All 7 webhooks fail after TLS cert corruption; cert-manager or operator restores cert within 120s.

<details>
<summary>Experiment YAML</summary>

```yaml
# NOTE: The Secret name 'odh-model-controller-webhook-cert' must be verified
# against the actual deployment. Controller-runtime webhook cert Secrets may
# follow different naming conventions depending on cert-manager or OLM setup.
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-webhook-cert-corrupt
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: Secret/odh-model-controller-webhook-cert
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: Secret
        name: odh-model-controller-webhook-cert
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: odh-model-controller-webhook-cert
      key: tls.crt
      value: corrupted
      resourceType: Secret
    ttl: "300s"
  hypothesis:
    description: >-
      All 7 webhooks fail after TLS cert corruption; cert-manager or operator
      restores cert within 120s.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### odh-model-controller-webhook-disrupt

- **Type:** WebhookDisrupt
- **Danger Level:** high
- **Component:** odh-model-controller

When the validating webhook failurePolicy is weakened from Fail to Ignore, invalid resources can bypass admission validation. The chaos framework restores the original failurePolicy via TTL-based cleanup after 60s. During the disruption window, the controller itself remains operational but admission guardrails are absent.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: odh-model-controller-webhook-disrupt
spec:
  target:
    operator: odh-model-controller
    component: odh-model-controller
    resource: ValidatingWebhookConfiguration/validating.odh-model-controller.opendatahub.io
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: WebhookDisrupt
    dangerLevel: high
    parameters:
      webhookName: validating.odh-model-controller.opendatahub.io
      action: setFailurePolicy
      value: Ignore
    ttl: "60s"
  hypothesis:
    description: >-
      When the validating webhook failurePolicy is weakened from Fail to
      Ignore, invalid resources can bypass admission validation. The chaos
      framework restores the original failurePolicy via TTL-based cleanup
      after 60s. During the disruption window, the controller itself
      remains operational but admission guardrails are absent.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

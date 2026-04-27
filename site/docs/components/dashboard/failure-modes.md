# dashboard Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| ConfigDrift | high | config-drift.yaml | When the kube-rbac-proxy configuration is corrupted, the RBAC proxy sidecar shou... |
| NetworkPartition | medium | network-partition.yaml | When odh-dashboard pods are network-partitioned from the API server, the dashboa... |
| PodKill | low | pod-kill.yaml | When one odh-dashboard pod is killed, the remaining replica should continue serv... |
| QuotaExhaustion | medium | quota-exhaustion.yaml | Exhausting pod quota in the dashboard namespace should prevent new pods from bei... |
| RBACRevoke | high | rbac-revoke.yaml | When the odh-dashboard ClusterRoleBinding subjects are revoked, the dashboard sh... |
| CRDMutation | high | route-backend-disruption.yaml | Changing the Route backend service name to a non-existent service simulates rout... |
| CRDMutation | high | route-host-collision.yaml | Mutating the dashboard Route host to a non-matching domain simulates a host coll... |
| CRDMutation | high | route-host-deletion.yaml | Deleting the Route host field via null merge patch removes the host assignment f... |
| CRDMutation | high | route-shard-mismatch.yaml | Setting spec.host to a domain that does not match any configured IngressControll... |
| CRDMutation | high | route-tls-mutation.yaml | Changing the TLS termination mode from "edge" or "reencrypt" to "passthrough" fo... |

## Experiment Details

### dashboard-config-drift

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** odh-dashboard

When the kube-rbac-proxy configuration is corrupted, the RBAC proxy sidecar should reject or misconfigure authorization decisions. The dashboard pods may need to be restarted to pick up the restored config. The operator should detect the drift and reconcile.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-config-drift
spec:
  tier: 2
  target:
    operator: dashboard
    component: odh-dashboard
    resource: ConfigMap/kube-rbac-proxy-config
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: kube-rbac-proxy-config
        namespace: opendatahub
    timeout: "30s"
  injection:
    type: ConfigDrift
    dangerLevel: high
    parameters:
      name: kube-rbac-proxy-config
      key: config-file.yaml
      value: '{"authorization":{"static":[{"verb":"*","resource":"invalid"}]}}'
      resourceType: ConfigMap
    ttl: "300s"
  hypothesis:
    description: >-
      When the kube-rbac-proxy configuration is corrupted, the RBAC proxy
      sidecar should reject or misconfigure authorization decisions. The
      dashboard pods may need to be restarted to pick up the restored
      config. The operator should detect the drift and reconcile.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 2
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### dashboard-network-partition

- **Type:** NetworkPartition
- **Danger Level:** medium
- **Component:** odh-dashboard

When odh-dashboard pods are network-partitioned from the API server, the dashboard UI should become unavailable as the kube-rbac-proxy sidecar cannot verify authentication. Once the partition is removed, the dashboard should resume serving without manual intervention.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-network-partition
spec:
  tier: 2
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Deployment/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: app=odh-dashboard
    ttl: "300s"
  hypothesis:
    description: >-
      When odh-dashboard pods are network-partitioned from the API server,
      the dashboard UI should become unavailable as the kube-rbac-proxy
      sidecar cannot verify authentication. Once the partition is removed,
      the dashboard should resume serving without manual intervention.
    recoveryTimeout: 180s
  blastRadius:
    maxPodsAffected: 2
    allowedNamespaces:
      - opendatahub
```

</details>

### dashboard-pod-kill

- **Type:** PodKill
- **Danger Level:** low
- **Component:** odh-dashboard

When one odh-dashboard pod is killed, the remaining replica should continue serving traffic. Kubernetes should recreate the killed pod within the recovery timeout and the deployment should return to 2/2 ready replicas.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-pod-kill
spec:
  tier: 1
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Deployment/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: PodKill
    parameters:
      labelSelector: app=odh-dashboard
    count: 1
    ttl: "300s"
  hypothesis:
    description: >-
      When one odh-dashboard pod is killed, the remaining replica should
      continue serving traffic. Kubernetes should recreate the killed pod
      within the recovery timeout and the deployment should return to 2/2
      ready replicas.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### dashboard-quota-exhaustion

- **Type:** QuotaExhaustion
- **Danger Level:** medium
- **Component:** odh-dashboard

Exhausting pod quota in the dashboard namespace should prevent new pods from being created. The operator should handle quota errors gracefully and recover when the quota is removed.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-quota-exhaustion
spec:
  tier: 5
  target:
    operator: dashboard
    component: odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: QuotaExhaustion
    parameters:
      quotaName: "chaos-quota-dashboard"
      pods: "0"
    ttl: "120s"
  hypothesis:
    description: >-
      Exhausting pod quota in the dashboard namespace should prevent new pods
      from being created. The operator should handle quota errors gracefully
      and recover when the quota is removed.
    recoveryTimeout: 60s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

</details>

### dashboard-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** odh-dashboard

When the odh-dashboard ClusterRoleBinding subjects are revoked, the dashboard should lose access to cluster-scoped resources like storage classes, nodes, and namespaces. API calls from the UI should return 403 errors. Once permissions are restored, normal operation should resume without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-rbac-revoke
spec:
  tier: 4
  target:
    operator: dashboard
    component: odh-dashboard
    resource: ClusterRoleBinding/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: RBACRevoke
    dangerLevel: high
    parameters:
      bindingName: odh-dashboard
      bindingType: ClusterRoleBinding
    ttl: "60s"
  hypothesis:
    description: >-
      When the odh-dashboard ClusterRoleBinding subjects are revoked, the
      dashboard should lose access to cluster-scoped resources like storage
      classes, nodes, and namespaces. API calls from the UI should return
      403 errors. Once permissions are restored, normal operation should
      resume without restart.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 2
    allowDangerous: true
```

</details>

### dashboard-route-backend-disruption

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-dashboard

Changing the Route backend service name to a non-existent service simulates route admission denial at the backend level. The Route is still admitted by the router, but all requests return 503 because the backend cannot be found. The operator should detect the broken backend reference and reconcile the Route to point to the correct service. Expected verdict: Resilient if the operator restores the backend, Vulnerable if requests continue to fail.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-route-backend-disruption
spec:
  tier: 3
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Route/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "odh-dashboard"
      path: "spec.to.name"
      value: "chaos-nonexistent-service"
    ttl: "300s"
  hypothesis:
    description: >-
      Changing the Route backend service name to a non-existent service
      simulates route admission denial at the backend level. The Route
      is still admitted by the router, but all requests return 503
      because the backend cannot be found. The operator should detect
      the broken backend reference and reconcile the Route to point to
      the correct service. Expected verdict: Resilient if the operator
      restores the backend, Vulnerable if requests continue to fail.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### dashboard-route-host-collision

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-dashboard

Mutating the dashboard Route host to a non-matching domain simulates a host collision or DNS misconfiguration. The OpenShift router will re-evaluate the Route and may reject or de-admit it. The RHOAI operator should detect the Route drift and reconcile the host back to its correct value. Expected verdict: Resilient if the operator restores the Route, Vulnerable if the Route remains misconfigured.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-route-host-collision
spec:
  tier: 3
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Route/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "odh-dashboard"
      path: "spec.host"
      value: "chaos-collision.apps.cluster.invalid"
    ttl: "300s"
  hypothesis:
    description: >-
      Mutating the dashboard Route host to a non-matching domain simulates
      a host collision or DNS misconfiguration. The OpenShift router will
      re-evaluate the Route and may reject or de-admit it. The RHOAI
      operator should detect the Route drift and reconcile the host back
      to its correct value. Expected verdict: Resilient if the operator
      restores the Route, Vulnerable if the Route remains misconfigured.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### dashboard-route-host-deletion

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-dashboard

Deleting the Route host field via null merge patch removes the host assignment from the Route. The OpenShift router de-admits the Route since it has no host to serve, making the dashboard unreachable. The operator should detect the missing host and restore the Route to its original configuration. This indirectly tests status clearing: without a host, the router clears the Route's admission status. Expected verdict: Resilient if the operator restores the host, Vulnerable if the Route remains without a host.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-route-host-deletion
spec:
  tier: 3
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Route/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "odh-dashboard"
      path: "spec.host"
      value: "null"
    ttl: "300s"
  hypothesis:
    description: >-
      Deleting the Route host field via null merge patch removes the
      host assignment from the Route. The OpenShift router de-admits
      the Route since it has no host to serve, making the dashboard
      unreachable. The operator should detect the missing host and
      restore the Route to its original configuration. This indirectly
      tests status clearing: without a host, the router clears the
      Route's admission status. Expected verdict: Resilient if the
      operator restores the host, Vulnerable if the Route remains
      without a host.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### dashboard-route-shard-mismatch

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-dashboard

Setting spec.host to a domain that does not match any configured IngressController's domain simulates a router shard misconfiguration. Unlike host-collision (which uses a cluster-like domain), this targets a completely non-routable local domain. No IngressController will claim the Route, making the dashboard unreachable through the orphaned host. The operator should detect the Route drift and restore the original host. Expected verdict: Resilient if the operator restores the host, Vulnerable if the Route remains orphaned on a non-existent shard.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-route-shard-mismatch
spec:
  tier: 3
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Route/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "odh-dashboard"
      path: "spec.host"
      value: "dashboard.nonexistent-shard.local"
    ttl: "300s"
  hypothesis:
    description: >-
      Setting spec.host to a domain that does not match any configured
      IngressController's domain simulates a router shard misconfiguration.
      Unlike host-collision (which uses a cluster-like domain), this
      targets a completely non-routable local domain. No IngressController
      will claim the Route, making the dashboard unreachable through the
      orphaned host. The operator should detect the Route drift and
      restore the original host. Expected verdict: Resilient if the
      operator restores the host, Vulnerable if the Route remains
      orphaned on a non-existent shard.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>

### dashboard-route-tls-mutation

- **Type:** CRDMutation
- **Danger Level:** high
- **Component:** odh-dashboard

Changing the TLS termination mode from "edge" or "reencrypt" to "passthrough" forces the router to stop terminating TLS and forward encrypted traffic directly to the backend pod. Since the dashboard pod likely does not serve TLS on its own, this breaks HTTPS access. The operator should detect the TLS config drift and restore the correct termination mode. Expected verdict: Resilient if the operator reconciles the TLS settings, Vulnerable if the Route remains broken.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.operatorchaos.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-route-tls-mutation
spec:
  tier: 3
  target:
    operator: dashboard
    component: odh-dashboard
    resource: Route/odh-dashboard
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-dashboard
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"
  injection:
    type: CRDMutation
    dangerLevel: high
    parameters:
      apiVersion: "route.openshift.io/v1"
      kind: "Route"
      name: "odh-dashboard"
      path: "spec.tls.termination"
      value: "passthrough"
    ttl: "300s"
  hypothesis:
    description: >-
      Changing the TLS termination mode from "edge" or "reencrypt" to
      "passthrough" forces the router to stop terminating TLS and forward
      encrypted traffic directly to the backend pod. Since the dashboard
      pod likely does not serve TLS on its own, this breaks HTTPS access.
      The operator should detect the TLS config drift and restore the
      correct termination mode. Expected verdict: Resilient if the
      operator reconciles the TLS settings, Vulnerable if the Route
      remains broken.
    recoveryTimeout: 120s
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

</details>


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

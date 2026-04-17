# dashboard Failure Modes

## Coverage

| Injection Type | Danger | Experiment | Description |
|----------------|--------|------------|-------------|
| ConfigDrift | high | config-drift.yaml | When the kube-rbac-proxy configuration is corrupted, the RBAC proxy sidecar shou... |
| NetworkPartition | medium | network-partition.yaml | When odh-dashboard pods are network-partitioned from the API server, the dashboa... |
| PodKill | low | pod-kill.yaml | When one odh-dashboard pod is killed, the remaining replica should continue serv... |
| RBACRevoke | high | rbac-revoke.yaml | When the odh-dashboard ClusterRoleBinding subjects are revoked, the dashboard sh... |

## Experiment Details

### dashboard-config-drift

- **Type:** ConfigDrift
- **Danger Level:** high
- **Component:** odh-dashboard

When the kube-rbac-proxy configuration is corrupted, the RBAC proxy sidecar should reject or misconfigure authorization decisions. The dashboard pods may need to be restarted to pick up the restored config. The operator should detect the drift and reconcile.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-config-drift
spec:
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
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-network-partition
spec:
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
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-pod-kill
spec:
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

### dashboard-rbac-revoke

- **Type:** RBACRevoke
- **Danger Level:** high
- **Component:** odh-dashboard

When the odh-dashboard ClusterRoleBinding subjects are revoked, the dashboard should lose access to cluster-scoped resources like storage classes, nodes, and namespaces. API calls from the UI should return 403 errors. Once permissions are restored, normal operation should resume without restart.

<details>
<summary>Experiment YAML</summary>

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: dashboard-rbac-revoke
spec:
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


<!-- custom-start: known-issues -->
<!-- custom-end: known-issues -->

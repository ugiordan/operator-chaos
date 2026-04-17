# ConfigDrift

**Danger Level:** :material-shield-check: Low

Modifies a key in a ConfigMap or Secret to test configuration reconciliation.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `resourceType` | `string` | Yes | - | Target resource type: ConfigMap or Secret |
| `name` | `string` | Yes | - | Name of the ConfigMap or Secret |
| `key` | `string` | Yes | - | Key within the data map to modify |
| `value` | `string` | Yes | - | Value to set (replaces existing value) |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

## How It Works

ConfigDrift reads the target ConfigMap or Secret, saves the original value of the specified key, and overwrites it with the injected value. For ConfigMaps, the original value is stored in a rollback annotation on the resource itself. For Secrets, a separate rollback Secret is created (to avoid exposing sensitive data in annotations).

**API calls:**
1. `Get` the target ConfigMap or Secret
2. Store original value (annotation for ConfigMap, separate Secret for Secret)
3. `Update` the resource with the new value
4. On cleanup: `Get` the rollback data, restore original value, remove rollback metadata

**Cleanup:** Restores the original value from rollback storage. If the key did not originally exist, it is deleted.

**Crash safety:** Rollback data persists in Kubernetes (annotation or Secret). The `Revert` method can restore the original value even after a crash.

## Disruption Rubric

**Expected behavior on a healthy operator:**
The operator detects the configuration change and either: (a) reconciles the ConfigMap/Secret back to the expected state, or (b) adapts its behavior to the new configuration gracefully. The steady-state check should pass within `recoveryTimeout`.

**Contract violation indicators:**
- Operator does not detect the change (no reconciliation triggered)
- Operator crashes or enters error loop due to invalid configuration (indicates missing validation)
- Configuration is silently accepted with incorrect behavior (indicates missing validation)

**Collateral damage risks:**
- Low. Only the specified key in the specified resource is modified
- If the ConfigMap is mounted as a volume, pods may need restart to pick up changes (depends on how the operator reads config)
- Using `dangerLevel: high` with `allowDangerous: true` is required for config changes that could affect cluster-wide behavior

**Recovery expectations:**
- Recovery time: 1-30 seconds (depends on reconciliation interval)
- Reconcile cycles: 1 (detect drift, restore expected state)
- What "recovered" means: ConfigMap/Secret has correct values, operator functioning normally

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| codeflare | codeflare-config-drift | high | When the codeflare operator configuration is corrupted, new cluster configuratio... |
| dashboard | dashboard-config-drift | high | When the kube-rbac-proxy configuration is corrupted, the RBAC proxy sidecar shou... |
| kserve | kserve-isvc-config-corruption | high | When the deploy key in the inferenceservice-config ConfigMap is overwritten with... |
| llamastack | llamastack-config-drift | high | When the llamastack serving configuration is corrupted, new LLM deployments rece... |
| modelmesh | modelmesh-config-drift | high | When the modelmesh serving configuration is corrupted, new model deployments rec... |
| odh-model-controller | odh-model-controller-config-drift | high | When the inferenceservice-config ConfigMap is corrupted with an invalid deployme... |
| odh-model-controller | odh-model-controller-ingress-config-corruption | high | When the ingress key in inferenceservice-config is emptied, the odh-model-contro... |
| odh-model-controller | odh-model-controller-webhook-cert-corrupt | high | All 7 webhooks fail after TLS cert corruption; cert-manager or operator restores... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

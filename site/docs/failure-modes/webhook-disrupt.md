# WebhookDisrupt

**Danger Level:** :material-shield-remove: High

Modifies failure policies on a ValidatingWebhookConfiguration to test webhook resilience.

## Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `webhookName` | `string` | Yes | - | Name of the ValidatingWebhookConfiguration resource |
| `value` | `string` | Yes | - | New failure policy: Fail or Ignore |
| `ttl` | `duration` | No | `300s` | Auto-cleanup duration |

## How It Works

WebhookDisrupt reads the target ValidatingWebhookConfiguration, saves the original `failurePolicy` for each webhook entry, and sets all entries to the specified value. This is a cluster-scoped operation.

**API calls:**
1. `Get` the ValidatingWebhookConfiguration (cluster-scoped)
2. Store original per-webhook failure policies in rollback annotation
3. `Update` all webhook entries with new `failurePolicy`
4. On cleanup: restore original per-webhook policies from rollback annotation

**Cleanup:** Restores each webhook's original `failurePolicy`. Idempotent (safe to call multiple times).

**Crash safety:** Rollback annotation persists on the resource. `Revert` restores original policies.

## Disruption Rubric

**Expected behavior on a healthy operator:**
Setting `failurePolicy: Ignore` means webhook validation is skipped. The operator should still function correctly because webhooks are a defense-in-depth mechanism, not a required dependency. Setting `failurePolicy: Fail` when the webhook service is unavailable blocks all matching API requests.

**Contract violation indicators:**
- Invalid resources are created when webhook is set to Ignore (indicates webhook is the only validation)
- Operator becomes completely non-functional when webhook policy changes (indicates tight coupling)
- Webhook configuration is not reconciled back by the operator

**Collateral damage risks:**
- **Very high.** This is cluster-scoped. ALL namespaces are affected.
- Setting webhooks to Ignore allows potentially invalid resources cluster-wide
- Setting webhooks to Fail when service is down blocks API operations cluster-wide
- Requires `dangerLevel: high` and `allowDangerous: true`

**Recovery expectations:**
- Recovery time: 1-10 seconds (operator reconciles webhook configuration)
- Reconcile cycles: 1
- What "recovered" means: webhook has original `failurePolicy` restored

## Cross-Component Results

| Component | Experiment | Danger | Description |
|-----------|------------|--------|-------------|
| data-science-pipelines | data-science-pipelines-webhook-disrupt | high | When the pipeline version validating webhook failurePolicy is weakened from Fail... |
| kserve | kserve-isvc-validator-disrupt | high | When the ValidatingWebhookConfiguration for InferenceService has its failurePoli... |
| kueue | kueue-webhook-disrupt | high | When the kueue validating webhook failurePolicy is weakened from Fail to Ignore,... |
| model-registry | model-registry-webhook-disrupt | high | When the ModelRegistry validating webhook failurePolicy is weakened from Fail to... |
| modelmesh | modelmesh-webhook-disrupt | high | When the modelmesh ServingRuntime validating webhook failurePolicy is weakened f... |
| odh-model-controller | odh-model-controller-webhook-disrupt | high | When the validating webhook failurePolicy is weakened from Fail to Ignore, inval... |
| opendatahub-operator | opendatahub-operator-webhook-disrupt | high | When the validating webhook failurePolicy is weakened from Fail to Ignore, inval... |
| workbenches | workbenches-webhook-disrupt | high | When the notebook mutating webhook failurePolicy is weakened from Fail to Ignore... |

<!-- custom-start: notes -->
<!-- custom-end: notes -->

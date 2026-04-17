---
name: WebhookDisrupt
type: WebhookDisrupt
danger: High
description: Modifies failure policies on a ValidatingWebhookConfiguration to test webhook resilience.
spec_fields:
  - name: webhookName
    type: string
    required: true
    description: Name of the ValidatingWebhookConfiguration resource
  - name: value
    type: string
    required: true
    description: "New failure policy: Fail or Ignore"
  - name: ttl
    type: duration
    required: false
    default: "300s"
    description: Auto-cleanup duration
---

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

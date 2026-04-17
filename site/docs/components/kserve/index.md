# kserve

## Overview

| Property | Value |
|----------|-------|
| **Operator** | kserve |
| **Namespace** | kserve |
| **Repository** | [https://github.com/kserve/kserve](https://github.com/kserve/kserve) |
| **Components** | 4 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ConfigMap | 1 |
| DaemonSet | 1 |
| Deployment | 3 |
| Lease | 4 |
| Secret | 3 |
| ServiceAccount | 4 |
| **Total** | **16** |

## Components

### kserve-controller-manager

**Controller:** KServe

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | kserve-controller-manager | kserve |
| v1 | ConfigMap | inferenceservice-config | kserve |
| v1 | ServiceAccount | kserve-controller-manager | kserve |
| v1 | Secret | kserve-webhook-server-cert | kserve |
| coordination.k8s.io/v1 | Lease | kserve-controller-manager-leader-lock | kserve |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| inferenceservice.kserve-webhook-server.defaulter | mutating | `/mutate-serving-kserve-io-v1beta1-inferenceservice` |
| inferenceservice.kserve-webhook-server.pod-mutator | mutating | `/mutate-pods` |
| inferenceservice.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1beta1-inferenceservice` |
| trainedmodel.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1alpha1-trainedmodel` |
| inferencegraph.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1alpha1-inferencegraph` |
| clusterservingruntime.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1alpha1-clusterservingruntime` |
| servingruntime.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1alpha1-servingruntime` |

#### Finalizers
- `inferenceservice.finalizers`
- `trainedmodel.finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | kserve-controller-manager | kserve | Available |

Timeout: 60s

### llmisvc-controller-manager

**Controller:** KServe
**Dependencies:** kserve-controller-manager

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | llmisvc-controller-manager | kserve |
| v1 | ServiceAccount | llmisvc-controller-manager | kserve |
| v1 | Secret | llmisvc-webhook-server-cert | kserve |
| coordination.k8s.io/v1 | Lease | llminferenceservice-kserve-controller-manager | kserve |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| llminferenceservice.kserve-webhook-server.v1alpha1.validator | validating | `/validate-serving-kserve-io-v1alpha1-llminferenceservice` |
| llminferenceservice.kserve-webhook-server.v1alpha2.validator | validating | `/validate-serving-kserve-io-v1alpha2-llminferenceservice` |
| llminferenceserviceconfig.kserve-webhook-server.v1alpha1.validator | validating | `/validate-serving-kserve-io-v1alpha1-llminferenceserviceconfig` |
| llminferenceserviceconfig.kserve-webhook-server.v1alpha2.validator | validating | `/validate-serving-kserve-io-v1alpha2-llminferenceserviceconfig` |

#### Finalizers
- `serving.kserve.io/llmisvc-finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | llmisvc-controller-manager | kserve | Available |

Timeout: 60s

### kserve-localmodel-controller-manager

**Controller:** KServe
**Dependencies:** kserve-controller-manager

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | kserve-localmodel-controller-manager | kserve |
| v1 | ServiceAccount | kserve-localmodel-controller-manager | kserve |
| v1 | Secret | localmodel-webhook-server-cert | kserve |
| coordination.k8s.io/v1 | Lease | kserve-local-model-manager-leader-lock | kserve |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| localmodelcache.kserve-webhook-server.validator | validating | `/validate-serving-kserve-io-v1alpha1-localmodelcache` |

#### Finalizers
- `localmodel.kserve.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | kserve-localmodel-controller-manager | kserve | Available |

Timeout: 60s

### kserve-localmodelnode-agent

**Controller:** KServe
**Dependencies:** kserve-localmodel-controller-manager

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | DaemonSet | kserve-localmodelnode-agent | kserve |
| v1 | ServiceAccount | kserve-localmodelnode-agent | kserve |
| coordination.k8s.io/v1 | Lease | kserve-local-model-node-manager-leader-lock | kserve |

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| resourceExists | DaemonSet | kserve-localmodelnode-agent | kserve |  |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

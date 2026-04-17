# odh-model-controller

## Overview

| Property | Value |
|----------|-------|
| **Operator** | odh-model-controller |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/opendatahub-io/odh-model-controller](https://github.com/opendatahub-io/odh-model-controller) |
| **Components** | 1 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 1 |
| ConfigMap | 1 |
| Deployment | 1 |
| Lease | 1 |
| Secret | 1 |
| ServiceAccount | 1 |
| **Total** | **6** |

## Components

### odh-model-controller

**Controller:** DataScienceCluster
**Dependencies:** kserve

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | odh-model-controller | opendatahub |
| v1 | ConfigMap | inferenceservice-config | opendatahub |
| v1 | ServiceAccount | odh-model-controller | opendatahub |
| v1 | Secret | odh-model-controller-webhook-cert | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | odh-model-controller-rolebinding-opendatahub |  |
| coordination.k8s.io/v1 | Lease | odh-model-controller.opendatahub.io | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| mutating.pod.odh-model-controller.opendatahub.io | mutating | `/mutate--v1-pod` |
| minferencegraph-v1alpha1.odh-model-controller.opendatahub.io | mutating | `/mutate-serving-kserve-io-v1alpha1-inferencegraph` |
| minferenceservice-v1beta1.odh-model-controller.opendatahub.io | mutating | `/mutate-serving-kserve-io-v1beta1-inferenceservice` |
| validating.nim.account.odh-model-controller.opendatahub.io | validating | `/validate-nim-opendatahub-io-v1-account` |
| validating.llmisvc.odh-model-controller.opendatahub.io | validating | `/validate-serving-kserve-io-v1alpha1-llminferenceservice` |
| vinferencegraph-v1alpha1.odh-model-controller.opendatahub.io | validating | `/validate-serving-kserve-io-v1alpha1-inferencegraph` |
| validating.isvc.odh-model-controller.opendatahub.io | validating | `/validate-serving-kserve-io-v1beta1-inferenceservice` |

#### Finalizers
- `odh.inferenceservice.finalizers`
- `modelregistry.opendatahub.io/finalizer`
- `runtimes.opendatahub.io/nim-cleanup-finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | odh-model-controller | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

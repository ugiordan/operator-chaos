# model-registry

## Overview

| Property | Value |
|----------|-------|
| **Operator** | model-registry |
| **Namespace** | odh-model-registries |
| **Repository** | [https://github.com/opendatahub-io/model-registry-operator](https://github.com/opendatahub-io/model-registry-operator) |
| **Components** | 1 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 1 |
| Deployment | 1 |
| Lease | 1 |
| Service | 1 |
| ServiceAccount | 1 |
| **Total** | **5** |

## Components

### model-registry-operator

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | model-registry-operator-controller-manager | odh-model-registries |
| v1 | ServiceAccount | model-registry-operator-controller-manager | odh-model-registries |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | model-registry-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | 85f368d1.opendatahub.io | odh-model-registries |
| v1 | Service | model-registry-operator-controller-manager-metrics-service | odh-model-registries |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vmodelregistry.opendatahub.io | validating | `/validate-modelregistry-opendatahub-io-modelregistry` |
| mmodelregistry.opendatahub.io | mutating | `/mutate-modelregistry-opendatahub-io-modelregistry` |

#### Finalizers
- `modelregistry.opendatahub.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | model-registry-operator-controller-manager | odh-model-registries | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

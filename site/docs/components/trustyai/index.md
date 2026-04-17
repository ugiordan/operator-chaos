# trustyai

## Overview

| Property | Value |
|----------|-------|
| **Operator** | trustyai |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/trustyai-explainability/trustyai-service-operator](https://github.com/trustyai-explainability/trustyai-service-operator) |
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

### trustyai-service-operator-controller-manager

**Controller:** DataScienceCluster
**Dependencies:** modelmesh-controller

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | trustyai-service-operator-controller-manager | opendatahub |
| v1 | ServiceAccount | trustyai-service-operator-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | trustyai-service-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | trustyai-service-operator-controller-manager-leader-election | opendatahub |
| v1 | Service | trustyai-service-operator-controller-manager-metrics-service | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vtrustyaiservice.trustyai.opendatahub.io | validating | `/validate-trustyai-opendatahub-io-v1alpha1-trustyaiservice` |

#### Finalizers
- `trustyai.opendatahub.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | trustyai-service-operator-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

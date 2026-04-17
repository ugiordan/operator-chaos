# llamastack

## Overview

| Property | Value |
|----------|-------|
| **Operator** | llamastack |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/meta-llama/llama-stack-k8s-operator](https://github.com/meta-llama/llama-stack-k8s-operator) |
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
| Service | 1 |
| ServiceAccount | 1 |
| **Total** | **6** |

## Components

### llamastack-controller-manager

**Controller:** DataScienceCluster
**Dependencies:** kserve-controller-manager

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | llamastack-controller-manager | opendatahub |
| v1 | ServiceAccount | llamastack-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | llamastack-controller-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | llamastack-controller-manager-leader-election | opendatahub |
| v1 | Service | llamastack-controller-manager-metrics-service | opendatahub |
| v1 | ConfigMap | llamastack-serving-config | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vllamastackdistribution.meta.com | validating | `/validate-llamastack-meta-com-v1alpha1-llamastackdistribution` |

#### Finalizers
- `llamastack.meta.com/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | llamastack-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

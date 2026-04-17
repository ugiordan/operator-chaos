# ray

## Overview

| Property | Value |
|----------|-------|
| **Operator** | ray |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/ray-project/kuberay](https://github.com/ray-project/kuberay) |
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

### ray-operator-controller-manager

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | ray-operator-controller-manager | opendatahub |
| v1 | ServiceAccount | ray-operator-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | ray-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | ray-operator-controller-manager-leader-election | opendatahub |
| v1 | Service | ray-operator-controller-manager-metrics-service | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vraycluster.ray.io | validating | `/validate-ray-io-v1-raycluster` |

#### Finalizers
- `ray.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | ray-operator-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

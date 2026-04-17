# training-operator

## Overview

| Property | Value |
|----------|-------|
| **Operator** | training-operator |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/kubeflow/training-operator](https://github.com/kubeflow/training-operator) |
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

### training-operator-controller-manager

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | training-operator-controller-manager | opendatahub |
| v1 | ServiceAccount | training-operator-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | training-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | training-operator-controller-manager-leader-election | opendatahub |
| v1 | Service | training-operator-controller-manager-metrics-service | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vpytorchjob.kubeflow.org | validating | `/validate-kubeflow-org-v1-pytorchjob` |
| mpytorchjob.kubeflow.org | mutating | `/mutate-kubeflow-org-v1-pytorchjob` |

#### Finalizers
- `training-operator.kubeflow.org/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | training-operator-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

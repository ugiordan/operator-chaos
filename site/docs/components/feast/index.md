# feast

## Overview

| Property | Value |
|----------|-------|
| **Operator** | feast |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/feast-dev/feast](https://github.com/feast-dev/feast) |
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

### feast-operator-controller-manager

**Controller:** DataScienceCluster
**Dependencies:** data-science-pipelines-operator

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | feast-operator-controller-manager | opendatahub |
| v1 | ServiceAccount | feast-operator-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | feast-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | feast-operator-controller-manager-leader-election | opendatahub |
| v1 | Service | feast-operator-controller-manager-metrics-service | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vfeaturestore.feast.dev | validating | `/validate-feast-dev-v1alpha1-featurestore` |

#### Finalizers
- `feast.dev/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | feast-operator-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

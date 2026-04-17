# kueue

## Overview

| Property | Value |
|----------|-------|
| **Operator** | kueue |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/kubernetes-sigs/kueue](https://github.com/kubernetes-sigs/kueue) |
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

### kueue-controller-manager

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | kueue-controller-manager | opendatahub |
| v1 | ServiceAccount | kueue-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | kueue-controller-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | kueue-controller-manager-leader-election | opendatahub |
| v1 | Service | kueue-controller-manager-metrics-service | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| vworkload.kb.io | validating | `/validate-kueue-x-k8s-io-v1beta1-workload` |
| vclusterqueue.kb.io | validating | `/validate-kueue-x-k8s-io-v1beta1-clusterqueue` |
| vlocalqueue.kb.io | validating | `/validate-kueue-x-k8s-io-v1beta1-localqueue` |
| vresourceflavor.kb.io | validating | `/validate-kueue-x-k8s-io-v1beta1-resourceflavor` |
| mworkload.kb.io | mutating | `/mutate-kueue-x-k8s-io-v1beta1-workload` |

#### Finalizers
- `kueue.x-k8s.io/managed-resources`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | kueue-controller-manager | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

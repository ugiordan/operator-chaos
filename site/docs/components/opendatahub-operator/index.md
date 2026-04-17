# opendatahub-operator

## Overview

| Property | Value |
|----------|-------|
| **Operator** | opendatahub-operator |
| **Namespace** | opendatahub-operator-system |
| **Repository** | [https://github.com/opendatahub-io/opendatahub-operator](https://github.com/opendatahub-io/opendatahub-operator) |
| **Components** | 1 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 1 |
| Deployment | 1 |
| Lease | 1 |
| Secret | 1 |
| Service | 1 |
| ServiceAccount | 1 |
| **Total** | **6** |

## Components

### opendatahub-operator

**Controller:** DSCInitialization

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | opendatahub-operator-controller-manager | opendatahub-operator-system |
| v1 | ServiceAccount | opendatahub-operator-controller-manager | opendatahub-operator-system |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | opendatahub-operator-controller-manager-rolebinding |  |
| v1 | Secret | opendatahub-operator-controller-webhook-cert | opendatahub-operator-system |
| coordination.k8s.io/v1 | Lease | 07ed84f7.opendatahub.io | opendatahub-operator-system |
| v1 | Service | opendatahub-operator-webhook-service | opendatahub-operator-system |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| mutating.datasciencecluster.opendatahub.io | mutating | `/mutate-datasciencecluster-v1` |
| validating.datasciencecluster.opendatahub.io | validating | `/validate-datasciencecluster-v1` |
| validating.dscinitialization.opendatahub.io | validating | `/validate-dscinitialization-v1` |

#### Finalizers
- `platform.opendatahub.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | opendatahub-operator-controller-manager | opendatahub-operator-system | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

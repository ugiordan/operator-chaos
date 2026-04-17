# workbenches

## Overview

| Property | Value |
|----------|-------|
| **Operator** | workbenches |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/opendatahub-io/opendatahub-operator](https://github.com/opendatahub-io/opendatahub-operator) |
| **Components** | 2 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 1 |
| ConfigMap | 1 |
| Deployment | 2 |
| Secret | 1 |
| Service | 1 |
| ServiceAccount | 2 |
| **Total** | **8** |

## Components

### odh-notebook-controller

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | odh-notebook-controller-manager | opendatahub |
| v1 | ServiceAccount | odh-notebook-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | odh-notebook-controller-manager-rolebinding |  |
| v1 | Service | odh-notebook-controller-webhook-service | opendatahub |
| v1 | Secret | odh-notebook-controller-webhook-cert | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| notebooks.opendatahub.io | mutating | `/mutate-notebook-v1` |
| connection-notebook.opendatahub.io | mutating | `/platform-connection-notebook` |

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | odh-notebook-controller-manager | opendatahub | Available |

Timeout: 60s

### kf-notebook-controller

**Controller:** DataScienceCluster
**Dependencies:** odh-notebook-controller

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | kf-notebook-controller-deployment | opendatahub |
| v1 | ServiceAccount | kf-notebook-controller-service-account | opendatahub |
| v1 | ConfigMap | notebook-controller-culler-config | opendatahub |

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | kf-notebook-controller-deployment | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

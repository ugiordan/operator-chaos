# dashboard

## Overview

| Property | Value |
|----------|-------|
| **Operator** | dashboard |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/opendatahub-io/odh-dashboard](https://github.com/opendatahub-io/odh-dashboard) |
| **Components** | 1 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 3 |
| ConfigMap | 1 |
| Deployment | 1 |
| Route | 1 |
| Service | 1 |
| ServiceAccount | 1 |
| **Total** | **8** |

## Components

### odh-dashboard

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | odh-dashboard | opendatahub |
| v1 | ServiceAccount | odh-dashboard | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | odh-dashboard |  |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | odh-dashboard-auth-delegator |  |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | odh-dashboard-monitoring |  |
| v1 | Service | odh-dashboard | opendatahub |
| v1 | ConfigMap | kube-rbac-proxy-config | opendatahub |
| route.openshift.io/v1 | Route | odh-dashboard | opendatahub |

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | odh-dashboard | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

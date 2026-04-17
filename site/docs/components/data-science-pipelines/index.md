# data-science-pipelines

## Overview

| Property | Value |
|----------|-------|
| **Operator** | data-science-pipelines |
| **Namespace** | opendatahub |
| **Repository** | [https://github.com/opendatahub-io/data-science-pipelines-operator](https://github.com/opendatahub-io/data-science-pipelines-operator) |
| **Components** | 2 |
| **Reconcile Timeout** | 300s |
| **Max Reconcile Cycles** | 10 |

## Resource Summary

| Kind | Count |
|------|-------|
| ClusterRoleBinding | 1 |
| ConfigMap | 1 |
| Deployment | 2 |
| Lease | 1 |
| Secret | 1 |
| ServiceAccount | 2 |
| **Total** | **8** |

## Components

### data-science-pipelines-operator

**Controller:** DataScienceCluster

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | data-science-pipelines-operator-controller-manager | opendatahub |
| v1 | ConfigMap | dspo-config | opendatahub |
| v1 | ServiceAccount | data-science-pipelines-operator-controller-manager | opendatahub |
| rbac.authorization.k8s.io/v1 | ClusterRoleBinding | data-science-pipelines-operator-manager-rolebinding |  |
| coordination.k8s.io/v1 | Lease | f9eb95d5.opendatahub.io | opendatahub |

#### Finalizers
- `datasciencepipelinesapplications.opendatahub.io/finalizer`

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | data-science-pipelines-operator-controller-manager | opendatahub | Available |

Timeout: 60s

### ds-pipelines-webhook

**Controller:** DataScienceCluster
**Dependencies:** data-science-pipelines-operator

#### Managed Resources

| API Version | Kind | Name | Namespace |
|-------------|------|------|-----------|
| apps/v1 | Deployment | ds-pipelines-webhook | opendatahub |
| v1 | ServiceAccount | ds-pipelines-webhook | opendatahub |
| v1 | Secret | ds-pipelines-webhook-tls | opendatahub |

#### Webhooks

| Name | Type | Path |
|------|------|------|
| validating.pipelineversions.pipelines.kubeflow.org | validating | `/webhooks/validate-pipelineversion` |
| mutating.pipelineversions.pipelines.kubeflow.org | mutating | `/webhooks/mutate-pipelineversion` |

#### Steady-State Checks

| Type | Kind | Name | Namespace | Condition |
|------|------|------|-----------|-----------|
| conditionTrue | Deployment | ds-pipelines-webhook | opendatahub | Available |

Timeout: 60s


<!-- custom-start: notes -->
<!-- custom-end: notes -->

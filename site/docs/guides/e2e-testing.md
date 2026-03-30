# End-to-End Testing Guide

Step-by-step guide for running chaos experiments against **odh-model-controller** on a live OpenShift/Kubernetes cluster. This component manages InferenceService lifecycle, model serving, and NIM accounts --- making it a critical target for resilience testing.

## Prerequisites

- OpenShift/Kubernetes cluster with OpenDataHub installed
- `cluster-admin` RBAC (experiments perform destructive operations)
- `odh-chaos` CLI built and in your PATH:

```bash
go install github.com/opendatahub-io/odh-platform-chaos/cmd/odh-chaos@latest
```

- Verify the target component is running:

```bash
kubectl get deployment odh-model-controller -n opendatahub
kubectl get pods -l control-plane=odh-model-controller -n opendatahub
```

## Step 1: Create the Knowledge Model

Save this as `knowledge/odh-model-controller.yaml`:

```yaml
operator:
  name: opendatahub-operator
  namespace: opendatahub

components:
  - name: odh-model-controller
    controller: DataScienceCluster
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        labels:
          control-plane: odh-model-controller
        expectedSpec:
          replicas: 1
      - apiVersion: v1
        kind: ServiceAccount
        name: odh-model-controller
        namespace: opendatahub
      - apiVersion: v1
        kind: ConfigMap
        name: inferenceservice-config
        namespace: opendatahub
    webhooks:
      - name: validating.odh-model-controller.opendatahub.io
        type: validating
        path: /validate
    finalizers:
      - odh.inferenceservice.finalizers
    steadyState:
      checks:
        - type: conditionTrue
          apiVersion: apps/v1
          kind: Deployment
          name: odh-model-controller
          namespace: opendatahub
          conditionType: Available
        - type: resourceExists
          apiVersion: v1
          kind: ConfigMap
          name: inferenceservice-config
          namespace: opendatahub
      timeout: "60s"

recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
```

Validate it:

```bash
odh-chaos validate knowledge/odh-model-controller.yaml --knowledge
```

Run pre-flight checks against the live cluster:

```bash
odh-chaos preflight --knowledge knowledge/odh-model-controller.yaml
```

## Step 2: Create Experiment Suite

Create a directory `experiments/odh-model-controller/` with one YAML per injection type.

### 2.1 PodKill --- Kill Controller Pods

**Danger: low** | Tests: pod restart and reconciliation loop recovery

Save as `experiments/odh-model-controller/01-podkill.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-podkill
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: PodKill
    parameters:
      labelSelector: "control-plane=odh-model-controller"
    count: 1
    dangerLevel: low
  hypothesis:
    description: "Killing odh-model-controller pod should trigger Deployment restart; controller should be Available within 60s"
    recoveryTimeout: "60s"
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

**Expected verdict**: Resilient --- the Deployment controller restarts the pod, and the operator becomes Available quickly.

### 2.2 ConfigDrift --- Corrupt InferenceService Config

**Danger: medium** | Tests: operator detection and restoration of configuration

Save as `experiments/odh-model-controller/02-configdrift.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-configdrift
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: ConfigDrift
    parameters:
      name: inferenceservice-config
      key: deploy
      value: '{"defaultDeploymentMode":"INVALID"}'
    dangerLevel: medium
  hypothesis:
    description: "Corrupting inferenceservice-config should be detected; operator should restore the ConfigMap to its expected state"
    recoveryTimeout: "120s"
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: v1
        kind: ConfigMap
        name: inferenceservice-config
        namespace: opendatahub
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

**Expected verdict**: Resilient if the parent operator (DataScienceCluster) reconciles the ConfigMap back. Degraded if recovery is slow or requires manual intervention.

### 2.3 NetworkPartition --- Isolate Controller Pods

**Danger: medium** | Tests: recovery from network isolation via NetworkPolicy

Save as `experiments/odh-model-controller/03-networkpartition.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-networkpartition
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: NetworkPartition
    parameters:
      labelSelector: "control-plane=odh-model-controller"
    ttl: "30s"
    dangerLevel: medium
  hypothesis:
    description: "Isolating odh-model-controller from the API server should cause temporary errors; after NetworkPolicy removal, controller should recover"
    recoveryTimeout: "120s"
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

**Expected verdict**: Resilient --- after the deny-all NetworkPolicy is removed (TTL expiry), the controller reconnects to the API server and resumes reconciliation.

### 2.4 CRDMutation --- Mutate an InferenceService

**Danger: medium** | Tests: operator detection and correction of CRD field drift

Save as `experiments/odh-model-controller/04-crdmutation.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-crdmutation
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
    resource: "InferenceService/my-model"
  injection:
    type: CRDMutation
    parameters:
      apiVersion: serving.kserve.io/v1beta1
      kind: InferenceService
      name: my-model
      field: replicas
      value: "0"
    dangerLevel: medium
  hypothesis:
    description: "Mutating InferenceService replicas to 0 should be corrected by the controller back to the desired state"
    recoveryTimeout: "120s"
  steadyState:
    checks:
      - type: resourceExists
        apiVersion: serving.kserve.io/v1beta1
        kind: InferenceService
        name: my-model
        namespace: opendatahub
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

> **Note**: Replace `my-model` with the name of an actual InferenceService deployed in your cluster. List available InferenceServices:
> ```bash
> kubectl get inferenceservice -n opendatahub
> ```

**Expected verdict**: Resilient if the controller restores the field. Degraded if recovery is slow.

### 2.5 FinalizerBlock --- Block Deployment Deletion

**Danger: medium** | Tests: operator behavior when a managed resource has a blocking finalizer

Save as `experiments/odh-model-controller/05-finalizerblock.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-finalizerblock
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: FinalizerBlock
    parameters:
      kind: Deployment
      name: odh-model-controller
    dangerLevel: medium
  hypothesis:
    description: "Adding a blocking finalizer to the Deployment should not prevent the operator from functioning; finalizer removal should allow normal cleanup"
    recoveryTimeout: "120s"
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
```

**Expected verdict**: Resilient --- the finalizer is added and removed cleanly; the operator continues to function normally.

### 2.6 WebhookDisrupt --- Disrupt Admission Webhook

**Danger: high** | Tests: recovery from webhook failure policy change

Save as `experiments/odh-model-controller/06-webhookdisrupt.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-webhookdisrupt
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: WebhookDisrupt
    parameters:
      webhookName: validating.odh-model-controller.opendatahub.io
      action: setFailurePolicy
    dangerLevel: high
  hypothesis:
    description: "Changing the webhook failure policy should be detected; the operator should restore the original policy"
    recoveryTimeout: "120s"
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

> **Important**: This is a high-danger injection --- `allowDangerous: true` is required. The webhook failure policy change can affect all admission requests in the cluster until restored.

**Expected verdict**: Resilient if the parent operator reconciles the webhook configuration back. Degraded if manual intervention is needed.

### 2.7 RBACRevoke --- Revoke Controller Permissions

**Danger: high** | Tests: recovery from RBAC permission loss

Save as `experiments/odh-model-controller/07-rbacrevoke.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: omc-rbacrevoke
  labels:
    chaos.opendatahub.io/component: odh-model-controller
    chaos.opendatahub.io/suite: e2e
spec:
  target:
    operator: opendatahub-operator
    component: odh-model-controller
  injection:
    type: RBACRevoke
    parameters:
      bindingName: odh-model-controller-rolebinding-opendatahub
      bindingType: ClusterRoleBinding
    dangerLevel: high
  hypothesis:
    description: "Revoking the controller's ClusterRoleBinding should cause permission errors; the parent operator should restore the binding"
    recoveryTimeout: "180s"
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: odh-model-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "60s"
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    allowDangerous: true
```

> **Important**: This is a high-danger injection --- `allowDangerous: true` is required. RBAC revocation causes the controller to lose all permissions until the binding is restored.

**Expected verdict**: Resilient if the parent operator (DataScienceCluster) restores the ClusterRoleBinding. Degraded if recovery is slow (> 180s).

## Step 3: Validate All Experiments

```bash
# Validate each experiment
for f in experiments/odh-model-controller/*.yaml; do
  echo "Validating $f..."
  odh-chaos validate "$f"
done
```

## Step 4: Dry Run

Test the full experiment lifecycle without injecting any faults:

```bash
odh-chaos suite experiments/odh-model-controller/ \
  --knowledge knowledge/odh-model-controller.yaml \
  --dry-run \
  --report-dir results/dry-run/
```

Review the dry-run results to ensure all experiments load correctly and steady-state checks pass.

## Step 5: Execute the Suite

Run all experiments sequentially (recommended for first run):

```bash
odh-chaos suite experiments/odh-model-controller/ \
  --knowledge knowledge/odh-model-controller.yaml \
  --report-dir results/live/ \
  --timeout 10m
```

Or run with distributed locking (for shared clusters):

```bash
odh-chaos suite experiments/odh-model-controller/ \
  --knowledge knowledge/odh-model-controller.yaml \
  --report-dir results/live/ \
  --timeout 10m \
  --distributed-lock
```

## Step 6: Review Results

Generate a summary report:

```bash
odh-chaos report results/live/ --format summary
```

Generate JUnit XML for CI/CD integration:

```bash
odh-chaos report results/live/ --format junit --output results/junit/
```

### Interpreting Verdicts

| Experiment | Expected Verdict | What it Means |
|-----------|-----------------|---------------|
| PodKill | Resilient | Deployment controller restarts pod, operator recovers |
| ConfigDrift | Resilient/Degraded | Parent operator restores ConfigMap (may be slow) |
| NetworkPartition | Resilient | Controller reconnects after NetworkPolicy removal |
| CRDMutation | Resilient/Degraded | Controller restores mutated CRD field |
| FinalizerBlock | Resilient | Operator functions despite blocking finalizer |
| WebhookDisrupt | Resilient/Degraded | Parent operator restores webhook config |
| RBACRevoke | Resilient/Degraded | Parent operator restores ClusterRoleBinding |

A **Degraded** verdict is not a failure --- it means recovery happened but was slower than expected or required extra reconcile cycles. Investigate by checking `recoveryTime` and `reconcileCycles` in the experiment results.

A **Failed** verdict means the operator did not restore the resource to its expected state within the timeout. This is a real resilience issue that should be investigated.

## Step 7: Cleanup

If any experiment left artifacts behind (e.g., due to a crash or timeout):

```bash
odh-chaos clean --namespace opendatahub
```

> **Note**: The `--namespace` flag scopes cleanup to namespace-scoped resources (NetworkPolicies, Leases, ConfigMaps, Secrets, Deployments) in the specified namespace, but also cleans cluster-scoped resources (ClusterRoles, ClusterRoleBindings, ValidatingWebhookConfigurations) with chaos metadata regardless of namespace.

To continuously watch for and clean stale artifacts:

```bash
odh-chaos clean --namespace opendatahub --watch --interval 30s
```

## Running Individual Experiments

You can run any single experiment instead of the full suite:

```bash
# Run just the PodKill experiment
odh-chaos run experiments/odh-model-controller/01-podkill.yaml \
  --knowledge knowledge/odh-model-controller.yaml

# Run with verbose output for debugging
odh-chaos run experiments/odh-model-controller/01-podkill.yaml \
  --knowledge knowledge/odh-model-controller.yaml \
  --verbose
```

## Adding Cross-Component Detection

To detect collateral damage on components that depend on odh-model-controller, add knowledge models for dependent operators:

```bash
# Knowledge files for kserve and odh-model-controller
# (llmisvc-controller-manager is a component within kserve.yaml)
ls knowledge/
  odh-model-controller.yaml
  kserve.yaml

# Run with --knowledge-dir to enable dependency graph
odh-chaos run experiments/odh-model-controller/01-podkill.yaml \
  --knowledge-dir knowledge/
```

Collateral findings appear in the experiment report and can downgrade a `Resilient` verdict to `Degraded`.

# Custom Failure Modes

This guide covers how to extend odh-platform-chaos with custom failure modes. There are two paths depending on your use case:

1. **YAML Composition (no code)**: Write custom experiments using existing injection types
2. **Go Plugin Development**: Add entirely new injection types to the framework

Most users will use YAML composition. Go plugins are only needed when the fault you want to inject isn't expressible through existing injection types.

---

## Path A: YAML Composition (No Code Required)

This is the primary extensibility path. You can create complex, custom chaos experiments by composing the 8 built-in injection types with different parameters, targets, and steady-state checks.

### Built-in Injection Types

The framework provides 8 injection types out of the box:

| Type | What It Does | Danger Level |
|------|--------------|--------------|
| `PodKill` | Force-delete pods matching a label selector | Low |
| `NetworkPartition` | Inject NetworkPolicy to isolate pods | Medium |
| `ConfigDrift` | Modify ConfigMap or Secret keys | Low to High |
| `CRDMutation` | Modify custom resource fields | Medium to High |
| `WebhookDisrupt` | Corrupt webhook configurations | High |
| `RBACRevoke` | Remove RBAC permissions | Medium |
| `FinalizerBlock` | Add blocking finalizers to resources | Medium |
| `ClientFault` | Inject API server request failures | High |

See the [Failure Modes](index.md) reference for full details on each type.

### Experiment YAML Structure

Every experiment follows this structure:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: my-custom-experiment
  labels:
    component: my-component
    severity: standard
spec:
  # What to target
  target:
    operator: opendatahub-operator
    component: my-component
    resource: Deployment/my-controller

  # What fault to inject
  injection:
    type: PodKill
    parameters:
      signal: "SIGKILL"
      labelSelector: "app=my-controller"
    count: 1
    ttl: "300s"

  # What "healthy" looks like
  steadyState:
    checks:
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: my-controller
        namespace: opendatahub
        conditionType: Available
    timeout: "30s"

  # What you expect to happen
  hypothesis:
    description: "Controller recovers from pod termination within 60s"
    recoveryTimeout: "60s"

  # Safety limits
  blastRadius:
    maxPodsAffected: 1
    allowedNamespaces:
      - opendatahub
    dryRun: false
```

### Writing Custom Steady-State Checks

Steady-state checks define what "healthy" means for your component. There are three types:

#### 1. `conditionTrue` Checks

Verify that a Kubernetes resource has a specific condition set to `True`:

```yaml
steadyState:
  checks:
    - type: conditionTrue
      apiVersion: apps/v1
      kind: Deployment
      name: my-controller
      namespace: opendatahub
      conditionType: Available
```

Common conditions:
- Deployment: `Available`, `Progressing`
- StatefulSet: `Available`
- Pod: `Ready`, `ContainersReady`
- Custom resources: any condition defined in the CRD

#### 2. `podReady` Checks

Verify that pods matching a label selector are ready:

```yaml
steadyState:
  checks:
    - type: podReady
      namespace: opendatahub
      labelSelector: "app=my-controller"
      minReadyPods: 1
```

#### 3. `customCommand` Checks

Run arbitrary commands to verify state:

```yaml
steadyState:
  checks:
    - type: customCommand
      command: "kubectl get inferenceservice -n test-ns my-model -o jsonpath='{.status.url}'"
      expectedOutput: "http://my-model.test-ns.svc.cluster.local"
```

Use custom commands sparingly. Prefer `conditionTrue` and `podReady` when possible, as they're more reliable and don't depend on shell availability.

### Parameterizing for Different Environments

The framework supports ODH and RHOAI deployments, which differ in namespaces, labels, and resource names.

#### Namespace Differences

- **ODH**: `opendatahub`
- **RHOAI**: `redhat-ods-applications` (for most components)

#### Using Knowledge Overlays

The `knowledge/` directory contains topology models for each operator. There are overlay directories for environment-specific variations:

```
knowledge/
  dashboard.yaml                   # ODH defaults
  rhoai/
    dashboard.yaml                 # RHOAI-specific overrides
  odh/v2.10/
    dashboard.yaml                 # ODH 2.10-specific overrides
  rhoai/v3.3/
    dashboard.yaml                 # RHOAI 3.3-specific overrides
```

When you specify `--knowledge-paths` to the CLI, later paths override earlier ones:

```bash
# ODH 2.10
odh-chaos run my-experiment.yaml \
  --knowledge knowledge/*.yaml \
  --knowledge knowledge/odh/v2.10/*.yaml

# RHOAI 3.3
odh-chaos run my-experiment.yaml \
  --knowledge knowledge/*.yaml \
  --knowledge knowledge/rhoai/*.yaml \
  --knowledge knowledge/rhoai/v3.3/*.yaml
```

#### Label Differences

Some components use different labels in RHOAI:

```yaml
# ODH
labelSelector: "app.kubernetes.io/part-of=odh-dashboard"

# RHOAI
labelSelector: "app.kubernetes.io/part-of=rhods-dashboard"
```

Check the knowledge YAML for your target component to see the correct labels.

### Chaining Experiments in Suites

The `odh-chaos suite` command runs multiple experiments sequentially or in parallel:

```bash
# Run all experiments in a directory
odh-chaos suite experiments/dashboard/ --namespace opendatahub

# Dry-run to validate without executing
odh-chaos suite experiments/dashboard/ --dry-run

# Run in parallel (max 4 concurrent)
odh-chaos suite experiments/dashboard/ --parallel 4

# Generate JUnit report
odh-chaos suite experiments/dashboard/ \
  --report-dir reports/ \
  --namespace opendatahub
```

**Custom suite directories** for targeted testing:

```
experiments/
  dashboard/
    podkill.yaml
    config-drift.yaml
    network-partition.yaml
  kserve/
    webhook-disrupt.yaml
    crd-mutation.yaml
  smoke-tests/           # Custom suite for CI
    critical-path.yaml
    basic-recovery.yaml
```

Suite-level features:
- Sequential execution (guarantees order)
- Parallel execution (max concurrency limit)
- Aggregate reporting (JUnit XML, JSON)
- Early termination on first failure
- Timeout per experiment

### Example: Complete Custom Experiment

Here's a full example testing ConfigMap corruption handling:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: custom-config-resilience
  labels:
    component: kserve
    severity: high
    test-type: config-validation
spec:
  target:
    operator: kserve
    component: kserve-controller-manager
    resource: ConfigMap/inferenceservice-config

  injection:
    type: ConfigDrift
    parameters:
      resourceType: ConfigMap
      name: inferenceservice-config
      key: deploy
      value: '{"defaultDeploymentMode": "InvalidMode"}'
    ttl: "300s"

  steadyState:
    checks:
      # Verify controller stays healthy
      - type: conditionTrue
        apiVersion: apps/v1
        kind: Deployment
        name: kserve-controller-manager
        namespace: opendatahub
        conditionType: Available
      # Verify pods don't crash
      - type: podReady
        namespace: opendatahub
        labelSelector: "control-plane=kserve-controller-manager"
        minReadyPods: 1
    timeout: "30s"

  hypothesis:
    description: >
      KServe controller should detect invalid config and either:
      (a) reconcile back to valid state, or
      (b) log validation errors without crashing.
      New InferenceService creations should fail with clear error messages.
    recoveryTimeout: "60s"

  blastRadius:
    maxPodsAffected: 5
    allowedNamespaces:
      - opendatahub
    dryRun: false
    allowDangerous: true  # Required for high danger level
```

Run it:

```bash
# Validate first
odh-chaos run experiments/custom-config-resilience.yaml --dry-run

# Execute
odh-chaos run experiments/custom-config-resilience.yaml \
  --namespace opendatahub \
  --knowledge knowledge/*.yaml \
  --verbose
```

### Using Existing Experiments as Templates

The `experiments/` directory contains production-tested experiments for all operators. Copy and modify them for your use case:

```bash
# Find experiments for your component
ls experiments/kserve/

# Copy and customize
cp experiments/kserve/kserve-podkill.yaml \
   experiments/custom/my-kserve-test.yaml

# Edit parameters, labels, steady-state checks
vi experiments/custom/my-kserve-test.yaml
```

---

## Path B: Go Plugin (New Injection Types)

Use this path only when you need to inject a fault that's not expressible through the 8 built-in types.

### When to Write a New Injector

**Use existing types when:**
- You can express the fault through PodKill, ConfigDrift, NetworkPartition, etc.
- The fault is a composition of multiple existing types

**Write a new injector when:**
- The fault requires custom API interactions not covered by existing types
- You're injecting a completely new category of failure (e.g., storage disruption, DNS corruption)
- You need specialized rollback logic that existing types don't provide

**Examples:**

| Fault | Use Existing Type | Or Write New Injector? |
|-------|------------------|------------------------|
| Kill pod with SIGTERM instead of SIGKILL | Use `PodKill` with `signal: "SIGTERM"` | No new code needed |
| Corrupt Secret value | Use `ConfigDrift` with `resourceType: Secret` | No new code needed |
| Block traffic to API server | Use `NetworkPartition` | No new code needed |
| Inject disk I/O errors on PVCs | Not covered by existing types | Yes, write new injector |
| Corrupt DNS entries in CoreDNS ConfigMap | Use `ConfigDrift` targeting CoreDNS | No new code needed |
| Simulate OOM kills | Not covered (needs cgroup manipulation) | Yes, write new injector |

### The Injector Interface

All injectors implement this interface:

```go
type Injector interface {
    // Validate checks that parameters are correct before injection
    Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error

    // Inject performs the fault and returns cleanup function + events
    Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error)

    // Revert restores the system to pre-injection state (crash-safe)
    Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error
}
```

**Key requirements:**

1. **Idempotency**: `Inject` and `Revert` must be idempotent (safe to call multiple times)
2. **Crash safety**: `Revert` must work even after a process crash (persist rollback data in Kubernetes)
3. **Blast radius enforcement**: Validate `blastRadius` limits before modifying resources
4. **Event emission**: Return structured events describing what was changed

### Full Development Guide

For the complete guide on implementing a new injection type, including:

- Step-by-step implementation walkthrough
- Code examples (ResourceQuotaDisrupt injector)
- Testing patterns
- Rollback data persistence
- CRD updates and registration
- Best practices and common patterns

See: [Adding Injection Types](../contributing/adding-injection-types.md)

---

## Best Practices

### 1. Start with PodKill

PodKill is the safest injection type and provides the fastest feedback loop:

```bash
# First experiment for any new component
odh-chaos run experiments/my-component-podkill.yaml --dry-run
odh-chaos run experiments/my-component-podkill.yaml --namespace opendatahub
```

If PodKill passes, move on to more disruptive types (ConfigDrift, NetworkPartition, etc.).

### 2. Always Use Dry-Run First

Dry-run mode validates experiments without executing them:

```bash
# Validates YAML syntax, injection parameters, blast radius
odh-chaos run my-experiment.yaml --dry-run

# For suites
odh-chaos suite experiments/dashboard/ --dry-run
```

Dry-run checks:
- YAML syntax
- Required parameters present
- Blast radius within limits
- Knowledge model exists for target operator
- Steady-state checks are well-formed

### 3. Set Appropriate Blast Radius Limits

Always constrain the blast radius to prevent runaway failures:

```yaml
blastRadius:
  maxPodsAffected: 1              # Limit pod deletions
  allowedNamespaces:
    - opendatahub                 # Only this namespace
  dryRun: false
  allowDangerous: false           # Require explicit opt-in for high-danger
```

For high-danger injections (ConfigDrift, WebhookDisrupt), you must set `allowDangerous: true`:

```yaml
injection:
  type: ConfigDrift
  # ... parameters ...

blastRadius:
  allowDangerous: true            # Required for high-danger types
  allowedNamespaces:
    - opendatahub
```

### 4. Test on Non-Production Clusters

**Never** run chaos experiments on production clusters unless you have:
- Tested the experiment on dev/staging first
- Reviewed the blast radius with your team
- Scheduled the test during a maintenance window
- Prepared rollback procedures

Use dedicated test clusters for experiment development.

### 5. Document Your Experiments

Add metadata to help others understand your experiments:

```yaml
metadata:
  name: descriptive-experiment-name
  labels:
    component: kserve
    severity: high
    test-type: config-validation
    owner: platform-team
    jira-ticket: RHOAIENG-12345
  annotations:
    description: |
      Tests KServe controller's handling of invalid configuration.
      Expected behavior: controller logs errors but does not crash.
    runbook: "https://docs.internal/chaos/kserve-config-tests"
```

---

## Troubleshooting

### Experiment Fails to Load

**Symptom**: `Error: failed to load experiment: ...`

**Causes**:
- YAML syntax errors (indentation, missing quotes)
- Invalid injection type
- Missing required parameters

**Fix**:
```bash
# Validate YAML syntax
yamllint my-experiment.yaml

# Check experiment structure
odh-chaos run my-experiment.yaml --dry-run --verbose
```

### Steady-State Check Fails Before Injection

**Symptom**: `Error: steady-state check failed before injection`

**Causes**:
- Component is not healthy to begin with
- Knowledge model specifies wrong resource names/namespaces
- Steady-state check timeout too short

**Fix**:
```bash
# Verify component is healthy
kubectl get deployment -n opendatahub
kubectl describe deployment my-controller -n opendatahub

# Check knowledge model matches reality
cat knowledge/my-operator.yaml

# Increase steady-state timeout
spec:
  steadyState:
    timeout: "60s"  # Increase from 30s
```

### Cleanup Doesn't Complete

**Symptom**: Resources still have chaos annotations/labels after experiment

**Causes**:
- Experiment interrupted (Ctrl+C during injection)
- Controller crashed during cleanup
- TTL expired but cleanup job didn't run

**Fix**:
```bash
# Manual cleanup
odh-chaos clean --namespace opendatahub

# Check for resources with chaos metadata
kubectl get all -n opendatahub \
  -l "chaos.opendatahub.io/injected=true"

# Remove chaos annotations manually (last resort)
kubectl annotate deployment my-controller \
  chaos.opendatahub.io/rollback-data- \
  -n opendatahub
```

### Permission Denied Errors

**Symptom**: `Error: ... is forbidden: User "..." cannot ...`

**Causes**:
- Insufficient RBAC permissions for odh-chaos CLI
- ServiceAccount missing required roles

**Fix**:
```bash
# Verify your permissions
kubectl auth can-i delete pods -n opendatahub
kubectl auth can-i update configmaps -n opendatahub

# Check odh-chaos ServiceAccount (if running in-cluster)
kubectl get clusterrole odh-chaos-role -o yaml
kubectl get clusterrolebinding odh-chaos-binding -o yaml
```

### Blast Radius Violations

**Symptom**: `Error: blast radius check failed: ...`

**Causes**:
- Injection would affect more pods than `maxPodsAffected`
- Target namespace not in `allowedNamespaces`
- High-danger injection without `allowDangerous: true`

**Fix**:
```yaml
# Adjust blast radius
blastRadius:
  maxPodsAffected: 5              # Increase limit
  allowedNamespaces:
    - opendatahub
    - test-namespace              # Add namespace
  allowDangerous: true            # Enable dangerous injections
```

---

## Next Steps

- Browse [built-in failure modes](index.md) for examples
- Explore [experiments directory](https://github.com/opendatahub-io/odh-platform-chaos/tree/main/experiments) for production-tested experiments
- Read [Adding Injection Types](../contributing/adding-injection-types.md) to write Go plugins
- Check [Architecture: Injection Engine](../architecture/injection-engine.md) to understand internals

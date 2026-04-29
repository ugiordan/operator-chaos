# Profiles

A profile is a set of knowledge models and experiments for a specific operator. Profiles let you reuse the operator-chaos framework with any Kubernetes operator without modifying the core tool.

## Built-in Profiles

The repository ships with two built-in profiles:

| Profile | Directory | Description |
|---------|-----------|-------------|
| ODH (default) | `knowledge/`, `experiments/` | OpenDataHub upstream components |
| RHOAI | `knowledge/rhoai/`, `experiments/rhoai/` | Red Hat OpenShift AI (downstream) |

The default (top-level) knowledge and experiments target ODH. The `rhoai` subdirectories contain RHOAI-specific variants with different namespaces, resource names, and deployment configurations.

## Example Profile: cert-manager

The `profiles/cert-manager/` directory demonstrates how to create a profile for a non-ODH operator:

```
profiles/cert-manager/
├── knowledge/
│   └── cert-manager.yaml      # 3 components: controller, webhook, cainjector
└── experiments/
    ├── controller-pod-kill.yaml
    ├── controller-network-partition.yaml
    └── rbac-revoke.yaml
```

Run it with:

```bash
# Preflight check
operator-chaos preflight --knowledge profiles/cert-manager/knowledge/cert-manager.yaml

# Single experiment
operator-chaos run profiles/cert-manager/experiments/controller-pod-kill.yaml \
  --profile cert-manager

# Full suite
operator-chaos suite profiles/cert-manager/experiments/ \
  --profile cert-manager --max-tier 2
```

## Using `--profile`

The `--profile` flag resolves knowledge directories automatically, so you don't need to pass `--knowledge-dir` manually.

```bash
# Without --profile (explicit paths):
operator-chaos suite experiments/rhoai/dashboard/ \
  --knowledge-dir knowledge/rhoai/

# With --profile (equivalent):
operator-chaos suite experiments/rhoai/dashboard/ \
  --profile rhoai
```

The flag searches two locations in order:

1. `profiles/<name>/knowledge/` (self-contained profile packs)
2. `knowledge/<name>/` (built-in subdirectory convention)

Explicit `--knowledge` or `--knowledge-dir` flags take precedence over `--profile`.

## Writing a Profile for Your Operator

### Step 1: Create the knowledge model

A knowledge model describes your operator's components, managed resources, and steady-state conditions. Create a YAML file following this structure:

```yaml
operator:
  name: my-operator
  namespace: my-operator-system
  repository: https://github.com/myorg/my-operator  # optional

components:
  - name: my-controller
    controller: Deployment
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: my-controller-manager
        namespace: my-operator-system
        labels:
          app: my-controller
        expectedSpec:
          replicas: 1
      - apiVersion: v1
        kind: ServiceAccount
        name: my-controller-manager
        namespace: my-operator-system
    steadyState:
      checks:
        - type: conditionTrue
          apiVersion: apps/v1
          kind: Deployment
          name: my-controller-manager
          namespace: my-operator-system
          conditionType: Available
      timeout: "60s"

recovery:
  reconcileTimeout: "120s"
  maxReconcileCycles: 5
```

Key fields:

- **operator.name**: Used to match experiments to knowledge via `spec.target.operator`
- **components[].managedResources**: Resources the operator creates and reconciles. Preflight checks these exist on the cluster.
- **components[].steadyState**: How to verify the component is healthy. Used as pre/post-injection checks.
- **recovery**: How long to wait for the operator to self-heal after fault injection

Validate with:

```bash
operator-chaos validate --knowledge path/to/knowledge.yaml
operator-chaos preflight --knowledge path/to/knowledge.yaml --local
```

### Step 2: Write experiments

Start with a Tier 1 PodKill experiment. Use `operator-chaos init` to generate a skeleton:

```bash
operator-chaos init --component my-controller --type PodKill
```

Then customize the generated YAML:

- Set `spec.target.operator` to match your knowledge model's `operator.name`
- Set `spec.steadyState` to match your knowledge model's steady-state checks
- Set `spec.injection.parameters.labelSelector` to target your operator's pods
- Set `spec.blastRadius.allowedNamespaces` to your operator's namespace

### Step 3: Organize into a profile

Place your files in a self-contained directory:

```
profiles/my-operator/
├── knowledge/
│   └── my-operator.yaml
└── experiments/
    ├── pod-kill.yaml
    └── network-partition.yaml
```

### Step 4: Validate and run

```bash
# Validate knowledge model
operator-chaos validate --knowledge profiles/my-operator/knowledge/my-operator.yaml

# Validate experiments
operator-chaos validate profiles/my-operator/experiments/pod-kill.yaml

# Preflight (requires cluster access)
operator-chaos preflight --knowledge profiles/my-operator/knowledge/my-operator.yaml

# Run experiments
operator-chaos suite profiles/my-operator/experiments/ --profile my-operator --max-tier 1
```

## Progressive Tier Adoption

Start with Tier 1 (PodKill) and work up:

| Tier | Types | Risk | Start Here |
|------|-------|------|------------|
| 1 | PodKill | Low | Yes: verify basic pod recovery |
| 2 | ConfigDrift, NetworkPartition | Low-Medium | After Tier 1 passes |
| 3 | CRDMutation, FinalizerBlock, LabelStomping | Medium | After Tier 2 passes |
| 4 | WebhookDisrupt, RBACRevoke | High | After Tier 3 passes |
| 5 | NamespaceDeletion, QuotaExhaustion | Very High | Production-grade operators only |
| 6 | Multi-fault scenarios | Very High | Mature operators with full test coverage |

```bash
# Start conservative
operator-chaos suite profiles/my-operator/experiments/ --profile my-operator --max-tier 1

# After Tier 1 passes, move up
operator-chaos suite profiles/my-operator/experiments/ --profile my-operator --max-tier 2
```

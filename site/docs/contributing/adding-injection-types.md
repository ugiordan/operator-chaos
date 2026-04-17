# Adding Injection Types

This guide walks you through implementing a new injection type from scratch. We'll create a `ResourceQuotaDisrupt` injector that modifies ResourceQuota limits to simulate resource exhaustion.

## Overview

Adding a new injection type involves:

1. Define the injection type constant
2. Implement the `Injector` interface
3. Register the injector with the registry
4. Add validation logic
5. Write tests
6. Update documentation

## Step 1: Define the Injection Type

### Add to CRD Enum

Edit `api/v1alpha1/types.go`:

```go
// InjectionType represents the type of fault injection.
// +kubebuilder:validation:Enum=PodKill;NetworkPartition;CRDMutation;ConfigDrift;WebhookDisrupt;RBACRevoke;FinalizerBlock;ClientFault;ResourceQuotaDisrupt
type InjectionType string

const (
    PodKill           InjectionType = "PodKill"
    NetworkPartition  InjectionType = "NetworkPartition"
    // ... existing types
    ResourceQuotaDisrupt InjectionType = "ResourceQuotaDisrupt"  // NEW
)

var validInjectionTypes = map[InjectionType]bool{
    PodKill:           true,
    NetworkPartition:  true,
    // ... existing types
    ResourceQuotaDisrupt: true,  // NEW
}
```

### Regenerate CRD Manifests

```bash
make generate
make manifests
```

This updates the CRD YAML with the new enum value.

## Step 2: Implement the Injector

Create `pkg/injection/resourcequota.go`:

```go
package injection

import (
    "context"
    "fmt"

    v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
    "github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/api/resource"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceQuotaDisruptInjector modifies ResourceQuota limits to simulate resource exhaustion.
type ResourceQuotaDisruptInjector struct {
    client client.Client
}

// NewResourceQuotaDisruptInjector creates a new ResourceQuotaDisruptInjector.
func NewResourceQuotaDisruptInjector(c client.Client) *ResourceQuotaDisruptInjector {
    return &ResourceQuotaDisruptInjector{client: c}
}

// Validate checks that required parameters are present and valid.
func (r *ResourceQuotaDisruptInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
    if spec.Parameters["name"] == "" {
        return fmt.Errorf("parameter 'name' is required")
    }
    if spec.Parameters["resource"] == "" {
        return fmt.Errorf("parameter 'resource' is required (e.g., 'requests.cpu', 'limits.memory')")
    }
    if spec.Parameters["value"] == "" {
        return fmt.Errorf("parameter 'value' is required (e.g., '100m', '512Mi')")
    }

    // Validate that value is a valid resource quantity
    if _, err := resource.ParseQuantity(spec.Parameters["value"]); err != nil {
        return fmt.Errorf("invalid resource quantity %q: %w", spec.Parameters["value"], err)
    }

    return nil
}

// Inject modifies the ResourceQuota and stores rollback data.
func (r *ResourceQuotaDisruptInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error) {
    key := types.NamespacedName{
        Name:      spec.Parameters["name"],
        Namespace: namespace,
    }
    resourceName := corev1.ResourceName(spec.Parameters["resource"])
    newValue, _ := resource.ParseQuantity(spec.Parameters["value"])

    // Fetch the ResourceQuota
    quota := &corev1.ResourceQuota{}
    if err := r.client.Get(ctx, key, quota); err != nil {
        return nil, nil, fmt.Errorf("getting ResourceQuota %s: %w", key, err)
    }

    // Save original limit
    var originalValue string
    if quota.Spec.Hard != nil {
        if origQty, ok := quota.Spec.Hard[resourceName]; ok {
            originalValue = origQty.String()
        }
    }

    // Build rollback data
    rollbackInfo := map[string]string{
        "resource":      spec.Parameters["resource"],
        "originalValue": originalValue,
    }
    rollbackStr, err := safety.WrapRollbackData(rollbackInfo)
    if err != nil {
        return nil, nil, fmt.Errorf("serializing rollback data: %w", err)
    }

    // Apply chaos metadata
    safety.ApplyChaosMetadata(quota, rollbackStr, string(v1alpha1.ResourceQuotaDisrupt))

    // Modify the quota
    if quota.Spec.Hard == nil {
        quota.Spec.Hard = make(corev1.ResourceList)
    }
    quota.Spec.Hard[resourceName] = newValue

    if err := r.client.Update(ctx, quota); err != nil {
        return nil, nil, fmt.Errorf("updating ResourceQuota %s: %w", key, err)
    }

    events := []v1alpha1.InjectionEvent{
        NewEvent(v1alpha1.ResourceQuotaDisrupt, key.String(), "modified",
            map[string]string{
                "resource": spec.Parameters["resource"],
                "value":    spec.Parameters["value"],
            }),
    }

    cleanup := func(ctx context.Context) error {
        return r.Revert(ctx, spec, namespace)
    }

    return cleanup, events, nil
}

// Revert restores the original ResourceQuota limit.
func (r *ResourceQuotaDisruptInjector) Revert(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) error {
    key := types.NamespacedName{
        Name:      spec.Parameters["name"],
        Namespace: namespace,
    }

    quota := &corev1.ResourceQuota{}
    if err := r.client.Get(ctx, key, quota); err != nil {
        if apierrors.IsNotFound(err) {
            return nil // Already gone
        }
        return fmt.Errorf("getting ResourceQuota %s for revert: %w", key, err)
    }

    // Check for rollback annotation
    rollbackStr, ok := quota.GetAnnotations()[safety.RollbackAnnotationKey]
    if !ok {
        return nil // Already reverted
    }

    var rollbackInfo map[string]string
    if err := safety.UnwrapRollbackData(rollbackStr, &rollbackInfo); err != nil {
        return fmt.Errorf("unwrapping rollback data: %w", err)
    }

    resourceName := corev1.ResourceName(rollbackInfo["resource"])

    // Restore original value
    if rollbackInfo["originalValue"] == "" {
        // Key didn't exist originally, remove it
        delete(quota.Spec.Hard, resourceName)
    } else {
        origQty, _ := resource.ParseQuantity(rollbackInfo["originalValue"])
        quota.Spec.Hard[resourceName] = origQty
    }

    // Remove chaos metadata
    safety.RemoveChaosMetadata(quota, string(v1alpha1.ResourceQuotaDisrupt))

    return r.client.Update(ctx, quota)
}
```

## Step 3: Register the Injector

Edit the registry initialization (typically in `cmd/controller/main.go` or a factory function):

```go
func NewDefaultRegistry(client client.Client) *injection.Registry {
    registry := injection.NewRegistry()

    registry.Register(v1alpha1.PodKill, injection.NewPodKillInjector(client))
    registry.Register(v1alpha1.NetworkPartition, injection.NewNetworkPartitionInjector(client))
    // ... existing types
    registry.Register(v1alpha1.ResourceQuotaDisrupt, injection.NewResourceQuotaDisruptInjector(client))  // NEW

    return registry
}
```

## Step 4: Add Validation

Create `pkg/injection/resourcequota_validate.go` (or add to `validate.go`):

```go
package injection

import (
    "fmt"

    v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
    "k8s.io/apimachinery/pkg/api/resource"
)

func validateResourceQuotaDisruptParams(spec v1alpha1.InjectionSpec) error {
    requiredParams := []string{"name", "resource", "value"}
    for _, param := range requiredParams {
        if spec.Parameters[param] == "" {
            return fmt.Errorf("parameter %q is required", param)
        }
    }

    // Validate resource name format
    resourceName := spec.Parameters["resource"]
    validResources := map[string]bool{
        "requests.cpu":    true,
        "requests.memory": true,
        "limits.cpu":      true,
        "limits.memory":   true,
        "pods":            true,
        "services":        true,
    }
    if !validResources[resourceName] {
        return fmt.Errorf("invalid resource %q; must be one of: requests.cpu, requests.memory, limits.cpu, limits.memory, pods, services", resourceName)
    }

    // Validate quantity format
    if _, err := resource.ParseQuantity(spec.Parameters["value"]); err != nil {
        return fmt.Errorf("invalid resource quantity %q: %w", spec.Parameters["value"], err)
    }

    return nil
}
```

Update the `Validate` method to use this helper:

```go
func (r *ResourceQuotaDisruptInjector) Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error {
    return validateResourceQuotaDisruptParams(spec)
}
```

## Step 5: Write Tests

Create `pkg/injection/resourcequota_test.go`:

```go
package injection

import (
    "context"
    "testing"

    v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
    "github.com/opendatahub-io/odh-platform-chaos/pkg/safety"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceQuotaDisrupt_Inject(t *testing.T) {
    scheme := runtime.NewScheme()
    _ = corev1.AddToScheme(scheme)

    // Create a ResourceQuota with initial limits
    originalQuota := &corev1.ResourceQuota{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-quota",
            Namespace: "test-ns",
        },
        Spec: corev1.ResourceQuotaSpec{
            Hard: corev1.ResourceList{
                corev1.ResourceRequestsCPU: resource.MustParse("4"),
            },
        },
    }

    client := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(originalQuota).
        Build()

    injector := NewResourceQuotaDisruptInjector(client)

    spec := v1alpha1.InjectionSpec{
        Type: v1alpha1.ResourceQuotaDisrupt,
        Parameters: map[string]string{
            "name":     "test-quota",
            "resource": "requests.cpu",
            "value":    "100m",
        },
    }

    ctx := context.Background()
    cleanup, events, err := injector.Inject(ctx, spec, "test-ns")
    if err != nil {
        t.Fatalf("Inject failed: %v", err)
    }

    // Verify quota was modified
    quota := &corev1.ResourceQuota{}
    client.Get(ctx, types.NamespacedName{Name: "test-quota", Namespace: "test-ns"}, quota)

    newLimit := quota.Spec.Hard[corev1.ResourceRequestsCPU]
    if newLimit.String() != "100m" {
        t.Errorf("Expected CPU limit to be 100m, got %s", newLimit.String())
    }

    // Verify rollback annotation exists
    rollbackStr, ok := quota.GetAnnotations()[safety.RollbackAnnotationKey]
    if !ok {
        t.Fatal("Rollback annotation not found")
    }

    var rollbackInfo map[string]string
    safety.UnwrapRollbackData(rollbackStr, &rollbackInfo)
    if rollbackInfo["originalValue"] != "4" {
        t.Errorf("Expected original value 4, got %s", rollbackInfo["originalValue"])
    }

    // Verify events
    if len(events) != 1 {
        t.Fatalf("Expected 1 event, got %d", len(events))
    }
    if events[0].Type != v1alpha1.ResourceQuotaDisrupt {
        t.Errorf("Expected event type ResourceQuotaDisrupt, got %s", events[0].Type)
    }

    // Test cleanup
    if err := cleanup(ctx); err != nil {
        t.Fatalf("Cleanup failed: %v", err)
    }

    // Verify quota restored
    client.Get(ctx, types.NamespacedName{Name: "test-quota", Namespace: "test-ns"}, quota)
    restoredLimit := quota.Spec.Hard[corev1.ResourceRequestsCPU]
    if restoredLimit.String() != "4" {
        t.Errorf("Expected CPU limit to be restored to 4, got %s", restoredLimit.String())
    }

    // Verify chaos metadata removed
    if _, ok := quota.GetAnnotations()[safety.RollbackAnnotationKey]; ok {
        t.Error("Rollback annotation should have been removed")
    }
}

func TestResourceQuotaDisrupt_Validate(t *testing.T) {
    injector := &ResourceQuotaDisruptInjector{}

    tests := []struct {
        name    string
        spec    v1alpha1.InjectionSpec
        wantErr bool
    }{
        {
            name: "valid spec",
            spec: v1alpha1.InjectionSpec{
                Parameters: map[string]string{
                    "name":     "test-quota",
                    "resource": "requests.cpu",
                    "value":    "100m",
                },
            },
            wantErr: false,
        },
        {
            name: "missing name",
            spec: v1alpha1.InjectionSpec{
                Parameters: map[string]string{
                    "resource": "requests.cpu",
                    "value":    "100m",
                },
            },
            wantErr: true,
        },
        {
            name: "invalid quantity",
            spec: v1alpha1.InjectionSpec{
                Parameters: map[string]string{
                    "name":     "test-quota",
                    "resource": "requests.cpu",
                    "value":    "invalid",
                },
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := injector.Validate(tt.spec, v1alpha1.BlastRadiusSpec{})
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

Run tests:

```bash
go test ./pkg/injection -v -run TestResourceQuotaDisrupt
```

## Step 6: Create an Example Experiment

Create `experiments/resourcequota-disrupt.yaml`:

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: resourcequota-cpu-exhaustion
spec:
  target:
    operator: namespace-operator
    component: resource-manager
  injection:
    type: ResourceQuotaDisrupt
    parameters:
      name: compute-resources
      resource: requests.cpu
      value: 100m  # Set to very low limit
  blastRadius:
    maxPodsAffected: 10
    allowedNamespaces: [test-namespace]
  steadyState:
    checks:
      - type: conditionTrue
        kind: ResourceQuota
        name: compute-resources
        namespace: test-namespace
        conditionType: Ready
  hypothesis:
    description: >
      Operator should detect ResourceQuota exhaustion and surface errors
      in logs or status conditions
    recoveryTimeout: 30s
```

## Step 7: Update Documentation

Add a metadata fragment to `hack/failure-mode-metadata/` which will be used to generate documentation pages in the failure-modes section:

````markdown
## ResourceQuotaDisrupt

Modifies ResourceQuota limits to simulate resource exhaustion.

### What It Does

- Changes hard limits in a ResourceQuota
- Triggers quota validation errors for new resources
- Tests operator behavior under resource constraints

### Spec Fields

```yaml
injection:
  type: ResourceQuotaDisrupt
  parameters:
    name: compute-resources      # Required: ResourceQuota name
    resource: requests.cpu       # Required: resource name
    value: 100m                  # Required: new limit
```

### Example Experiment

```yaml
apiVersion: chaos.opendatahub.io/v1alpha1
kind: ChaosExperiment
metadata:
  name: quota-exhaustion
spec:
  target:
    operator: my-operator
    component: controller
  injection:
    type: ResourceQuotaDisrupt
    parameters:
      name: compute-resources
      resource: requests.cpu
      value: 100m
  blastRadius:
    maxPodsAffected: 5
    allowedNamespaces: [test-namespace]
  hypothesis:
    description: Operator should handle quota errors gracefully
    recoveryTimeout: 60s
```

### What to Expect

- **During Injection**: ResourceQuota limit reduced
- **Recovery**: Original limit restored
- **Success Criteria**: Operator detects quota errors, does not crash
````

## Step 8: Test End-to-End

### 1. Build and Deploy

```bash
make generate
make manifests
kubectl apply -f config/crd/
```

### 2. Run Controller Locally

```bash
go run ./cmd/controller/main.go
```

### 3. Submit Experiment

```bash
kubectl apply -f experiments/resourcequota-disrupt.yaml
```

### 4. Observe Results

```bash
kubectl get chaosexperiment resourcequota-cpu-exhaustion -w
kubectl describe chaosexperiment resourcequota-cpu-exhaustion
```

## Best Practices

### 1. Idempotent Cleanup

Always check for rollback annotation existence:

```go
if rollbackStr, ok := resource.GetAnnotations()[safety.RollbackAnnotationKey]; !ok {
    return nil  // Already reverted
}
```

### 2. Comprehensive Validation

Validate all parameters early:

- Required fields
- Format (selectors, quantities, paths)
- Blast radius limits

### 3. Descriptive Events

Provide actionable details in injection events:

```go
NewEvent(injectionType, target, action, map[string]string{
    "namespace": namespace,
    "resource":  resourceName,
    "oldValue":  originalValue,
    "newValue":  newValue,
})
```

### 4. Error Context

Wrap errors with context:

```go
return fmt.Errorf("updating ResourceQuota %s/%s: %w", namespace, name, err)
```

### 5. Test Edge Cases

- Missing resources
- Already-modified resources (re-injection)
- Controller restart during injection
- Concurrent experiments

## Common Patterns

### Pattern: Modify Cluster-Scoped Resources

For cluster-scoped resources (ClusterRole, ValidatingWebhookConfiguration):

```go
// Skip namespace parameter
func (i *ClusterInjector) Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) {
    // Ignore namespace, use cluster-scoped key
    key := client.ObjectKey{Name: spec.Parameters["name"]}
    // ...
}
```

### Pattern: Create Temporary Resources

For injections that create new resources (like NetworkPolicy):

```go
// In Inject():
policy := &networkingv1.NetworkPolicy{...}
if err := client.Create(ctx, policy); err != nil {
    return nil, nil, err
}

cleanup := func(ctx context.Context) error {
    return client.Delete(ctx, policy)
}

// In Revert():
policy := &networkingv1.NetworkPolicy{...}
policy.Name = reconstructName(spec.Parameters)
if err := client.Delete(ctx, policy); client.IgnoreNotFound(err) != nil {
    return err
}
```

### Pattern: Multi-Resource Modification

For injections affecting multiple resources:

```go
rollbackInfo := map[string][]OriginalData{
    "webhooks": []OriginalData{...},
}
rollbackStr, _ := safety.WrapRollbackData(rollbackInfo)

// Apply to parent resource
safety.ApplyChaosMetadata(webhookConfig, rollbackStr, injectionType)
```

## Next Steps

- Review existing injectors in `pkg/injection/` for examples
- Read [Injection Engine Architecture](../architecture/injection-engine.md)
- Test your injector with [Development Setup](development-setup.md)
- Submit a pull request with your new injection type!

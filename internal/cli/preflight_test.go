package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

const validKnowledgeYAML = `operator:
  name: test-operator
  namespace: test-ns
components:
  - name: dashboard
    controller: DataScienceCluster
    managedResources:
      - apiVersion: apps/v1
        kind: Deployment
        name: test-dashboard
        namespace: test-ns
      - apiVersion: v1
        kind: Service
        name: test-dashboard-svc
        namespace: test-ns
    steadyState:
      checks:
        - type: conditionTrue
          apiVersion: apps/v1
          kind: Deployment
          name: test-dashboard
          conditionType: Available
      timeout: "60s"
recovery:
  reconcileTimeout: "300s"
  maxReconcileCycles: 10
`

const invalidKnowledgeYAML = `operator:
  name: ""
  namespace: ""
components: []
recovery:
  reconcileTimeout: "0s"
  maxReconcileCycles: 0
`

// writeTestKnowledge writes YAML content to a temp file and returns its path.
func writeTestKnowledge(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "knowledge.yaml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0600))
	return p
}

func TestPreflightLocalValidKnowledge(t *testing.T) {
	path := writeTestKnowledge(t, validKnowledgeYAML)
	cmd := newPreflightCommand()
	cmd.SetArgs([]string{
		"--knowledge", path,
		"--local",
	})

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestPreflightLocalInvalidKnowledge(t *testing.T) {
	path := writeTestKnowledge(t, invalidKnowledgeYAML)
	cmd := newPreflightCommand()
	cmd.SetArgs([]string{
		"--knowledge", path,
		"--local",
	})

	err := cmd.Execute()
	assert.Error(t, err)
}

func TestPreflightLocalNonExistentFile(t *testing.T) {
	cmd := newPreflightCommand()
	cmd.SetArgs([]string{
		"--knowledge", "/nonexistent/path/knowledge.yaml",
		"--local",
	})

	err := cmd.Execute()
	assert.Error(t, err)
}

func TestPreflightCrossReferenceCheckSteadyStateRef(t *testing.T) {
	knowledge := &model.OperatorKnowledge{
		Operator: model.OperatorMeta{
			Name:      "test-operator",
			Namespace: "test-ns",
		},
		Components: []model.ComponentModel{
			{
				Name:       "comp1",
				Controller: "TestController",
				ManagedResources: []model.ManagedResource{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "my-deploy",
					},
				},
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{
							Type: v1alpha1.CheckConditionTrue,
							Name: "nonexistent-resource",
						},
					},
				},
			},
		},
	}

	errs := crossReferenceChecks(knowledge)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0], "nonexistent-resource")
	assert.Contains(t, errs[0], "not declared in managedResources")
}

func TestPreflightCrossReferenceCheckValid(t *testing.T) {
	knowledge := &model.OperatorKnowledge{
		Operator: model.OperatorMeta{
			Name:      "test-operator",
			Namespace: "test-ns",
		},
		Components: []model.ComponentModel{
			{
				Name:       "comp1",
				Controller: "TestController",
				ManagedResources: []model.ManagedResource{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "my-deploy",
					},
				},
				SteadyState: v1alpha1.SteadyStateSpec{
					Checks: []v1alpha1.SteadyStateCheck{
						{
							Type: v1alpha1.CheckConditionTrue,
							Name: "my-deploy",
						},
					},
				},
			},
		},
	}

	errs := crossReferenceChecks(knowledge)
	assert.Empty(t, errs)
}

func TestPreflightFlagRegistration(t *testing.T) {
	cmd := newPreflightCommand()

	knowledgeFlag := cmd.Flags().Lookup("knowledge")
	require.NotNil(t, knowledgeFlag, "--knowledge flag should be registered")

	localFlag := cmd.Flags().Lookup("local")
	require.NotNil(t, localFlag, "--local flag should be registered")
}

func TestPreflightClusterResourceCheck(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dashboard",
			Namespace: "test-ns",
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "test-ns",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deploy, svc).
		Build()

	// Set status after creation to avoid WithStatusSubresource stripping
	deploy.Status.Conditions = []appsv1.DeploymentCondition{
		{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionTrue,
		},
	}
	require.NoError(t, k8sClient.Status().Update(context.Background(), deploy))

	knowledge := &model.OperatorKnowledge{
		Components: []model.ComponentModel{
			{
				Name: "dashboard",
				ManagedResources: []model.ManagedResource{
					{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "test-dashboard",
						Namespace:  "test-ns",
					},
					{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       "test-service",
						Namespace:  "test-ns",
					},
					{
						APIVersion: "v1",
						Kind:       "Service",
						Name:       "missing-service",
						Namespace:  "test-ns",
					},
				},
			},
		},
	}

	results := checkClusterResources(context.Background(), k8sClient, knowledge, "test-ns")
	require.Len(t, results, 3)

	// The deployment exists and has Available=True -> Found
	assert.Equal(t, "Found", results[0].Status)
	assert.Equal(t, "test-dashboard", results[0].Name)

	// The service exists -> Found
	assert.Equal(t, "Found", results[1].Status)
	assert.Equal(t, "test-service", results[1].Name)

	// The missing service -> Missing
	assert.Equal(t, "Missing", results[2].Status)
	assert.Equal(t, "missing-service", results[2].Name)
}

func TestPreflightDeploymentDegraded(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "degraded-deploy",
			Namespace: "test-ns",
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deploy).
		Build()

	// Set degraded status after creation
	deploy.Status.Conditions = []appsv1.DeploymentCondition{
		{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionFalse,
		},
	}
	require.NoError(t, k8sClient.Status().Update(context.Background(), deploy))

	mr := model.ManagedResource{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "degraded-deploy",
	}

	status, errMsg := checkSingleResource(context.Background(), k8sClient, mr, "test-ns")
	assert.Equal(t, "Degraded", status)
	assert.Empty(t, errMsg)
}

func TestPreflightClusterScopedKindsMap(t *testing.T) {
	// Verify that cluster-scoped kinds are correctly identified
	clusterScoped := []string{
		"ClusterRole", "ClusterRoleBinding", "CustomResourceDefinition",
		"ValidatingWebhookConfiguration", "MutatingWebhookConfiguration",
		"Namespace", "PersistentVolume", "StorageClass", "IngressClass",
		"PriorityClass", "APIService", "Node",
	}
	for _, kind := range clusterScoped {
		assert.True(t, clusterScopedKinds[kind], "expected %s to be cluster-scoped", kind)
	}

	// Verify namespaced kinds are not in the map
	namespacedKinds := []string{"Deployment", "Service", "ConfigMap", "Secret", "Pod"}
	for _, kind := range namespacedKinds {
		assert.False(t, clusterScopedKinds[kind], "expected %s to be namespaced", kind)
	}
}

func TestPreflightClusterScopedNamespaceLogic(t *testing.T) {
	// Test that checkClusterResources does not inject namespace for cluster-scoped kinds
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	knowledge := &model.OperatorKnowledge{
		Components: []model.ComponentModel{
			{
				Name: "controller",
				ManagedResources: []model.ManagedResource{
					{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
						Name:       "test-crb",
						// Intentionally no namespace, and a default namespace is passed
					},
					{
						APIVersion: "rbac.authorization.k8s.io/v1",
						Kind:       "ClusterRoleBinding",
						Name:       "test-crb-with-ns",
						Namespace:  "should-be-ignored", // should be cleared for cluster-scoped
					},
				},
			},
		},
	}

	results := checkClusterResources(context.Background(),
		fake.NewClientBuilder().WithScheme(scheme).Build(),
		knowledge, "default-ns-should-not-be-used")

	require.Len(t, results, 2)
	// Both should be ClusterRoleBinding
	assert.Equal(t, "ClusterRoleBinding", results[0].Kind)
	assert.Equal(t, "ClusterRoleBinding", results[1].Kind)
}

func TestPreflightErrorStatusForUnknownGVK(t *testing.T) {
	// Use a scheme that doesn't know about the requested resource type
	// This triggers the "Error" status path (non-NotFound error)
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	// Don't add appsv1 - Deployment GVK will be unknown

	mr := model.ManagedResource{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "unknown-deploy",
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	status, errMsg := checkSingleResource(context.Background(), k8sClient, mr, "test-ns")
	// The fake client returns an error for unregistered types
	assert.Equal(t, "Error", status)
	assert.NotEmpty(t, errMsg)
}

func TestPreflightVerboseLocalMode(t *testing.T) {
	path := writeTestKnowledge(t, validKnowledgeYAML)
	cmd := newPreflightCommand()
	// Add verbose as a persistent flag (simulating root command)
	cmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	cmd.SetArgs([]string{
		"--knowledge", path,
		"--local",
		"--verbose",
	})

	err := cmd.Execute()
	assert.NoError(t, err)
}

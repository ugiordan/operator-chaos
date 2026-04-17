package olm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetSubscriptionStatus(t *testing.T) {
	sub := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata":   map[string]interface{}{"name": "rhods-operator", "namespace": "redhat-ods-operator"},
			"status": map[string]interface{}{
				"installedCSV": "rhods-operator.v2.10.1",
				"currentCSV":   "rhods-operator.v3.3.1",
			},
		},
	}

	installed, current, err := getSubscriptionStatus(sub)
	require.NoError(t, err)
	assert.Equal(t, "rhods-operator.v2.10.1", installed)
	assert.Equal(t, "rhods-operator.v3.3.1", current)
}

func TestGetSubscriptionStatusMissing(t *testing.T) {
	sub := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata":   map[string]interface{}{"name": "test"},
		},
	}

	installed, current, err := getSubscriptionStatus(sub)
	require.NoError(t, err)
	assert.Empty(t, installed)
	assert.Empty(t, current)
}

func TestGetCSVPhase(t *testing.T) {
	csv := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "ClusterServiceVersion",
			"metadata":   map[string]interface{}{"name": "rhods-operator.v3.3.1"},
			"status":     map[string]interface{}{"phase": "Succeeded"},
		},
	}

	phase, err := getCSVPhase(csv)
	require.NoError(t, err)
	assert.Equal(t, "Succeeded", phase)
}

func TestGetInstallPlanPhase(t *testing.T) {
	ip := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "InstallPlan",
			"metadata":   map[string]interface{}{"name": "install-abc"},
			"spec":       map[string]interface{}{"approval": "Automatic"},
			"status":     map[string]interface{}{"phase": "Complete"},
		},
	}

	phase, err := getInstallPlanPhase(ip)
	require.NoError(t, err)
	assert.Equal(t, "Complete", phase)
}

func TestGetInstallPlanApproval(t *testing.T) {
	ip := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "InstallPlan",
			"metadata":   map[string]interface{}{"name": "install-abc"},
			"spec":       map[string]interface{}{"approval": "Manual"},
		},
	}

	approval, err := getInstallPlanApproval(ip)
	require.NoError(t, err)
	assert.Equal(t, "Manual", approval)
}

func TestGetPackageChannels(t *testing.T) {
	pm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "packages.operators.coreos.com/v1",
			"kind":       "PackageManifest",
			"metadata":   map[string]interface{}{"name": "rhods-operator"},
			"status": map[string]interface{}{
				"channels": []interface{}{
					map[string]interface{}{
						"name":       "stable-3.3",
						"currentCSV": "rhods-operator.v3.3.1",
						"currentCSVDesc": map[string]interface{}{
							"version": "3.3.1",
						},
					},
					map[string]interface{}{
						"name":       "stable-2.10",
						"currentCSV": "rhods-operator.v2.10.1",
						"currentCSVDesc": map[string]interface{}{
							"version": "2.10.1",
						},
					},
				},
			},
		},
	}

	channels, err := getPackageChannels(pm)
	require.NoError(t, err)
	require.Len(t, channels, 2)
	assert.Equal(t, "stable-3.3", channels[0].Name)
	assert.Equal(t, "3.3.1", channels[0].HeadVersion)
	assert.Equal(t, "rhods-operator.v3.3.1", channels[0].CSVName)
}

func TestGetSubscriptionInstallPlanRef(t *testing.T) {
	sub := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata":   map[string]interface{}{"name": "rhods-operator"},
			"status": map[string]interface{}{
				"installPlanRef": map[string]interface{}{
					"name":      "install-abc123",
					"namespace": "redhat-ods-operator",
				},
			},
		},
	}

	name, ns, err := getSubscriptionInstallPlanRef(sub)
	require.NoError(t, err)
	assert.Equal(t, "install-abc123", name)
	assert.Equal(t, "redhat-ods-operator", ns)
}

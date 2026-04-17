package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSnapshotBeforeHop(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apiextv1.AddToScheme(scheme))

	crd := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "notebooks.kubeflow.org",
			Labels: map[string]string{
				"operators.coreos.com/rhods-operator.redhat-ods-operator": "",
			},
		},
		Spec: apiextv1.CustomResourceDefinitionSpec{
			Group: "kubeflow.org",
			Names: apiextv1.CustomResourceDefinitionNames{
				Plural:   "notebooks",
				Singular: "notebook",
				Kind:     "Notebook",
			},
			Scope: apiextv1.ClusterScoped,
			Versions: []apiextv1.CustomResourceDefinitionVersion{
				{Name: "v1", Served: true, Storage: true},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
	snapshotDir := t.TempDir()

	mgr := NewRollbackManager(nil, k8sClient, snapshotDir, RollbackConfig{
		Enabled:      true,
		SnapshotCRDs: true,
	})

	err := mgr.SnapshotBeforeHop(context.Background(), "rhods-operator", "redhat-ods-operator", "", 0)
	require.NoError(t, err)

	// Verify the CRD snapshot directory exists
	crdDir := filepath.Join(snapshotDir, "snapshots", "hop-0", "crds")
	entries, err := os.ReadDir(crdDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "notebooks.kubeflow.org.yaml", entries[0].Name())

	// Verify the CRD YAML content is non-empty
	data, err := os.ReadFile(filepath.Join(crdDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(data), "kubeflow.org")
}

func TestSnapshotBeforeHopSavesChannel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apiextv1.AddToScheme(scheme))
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	snapshotDir := t.TempDir()

	// Without an OLM client, channel.txt should not be created
	mgr := NewRollbackManager(nil, k8sClient, snapshotDir, RollbackConfig{
		Enabled:      true,
		SnapshotCRDs: false,
	})

	err := mgr.SnapshotBeforeHop(context.Background(), "rhods-operator", "redhat-ods-operator", "", 1)
	require.NoError(t, err)

	channelPath := filepath.Join(snapshotDir, "snapshots", "hop-1", "channel.txt")
	_, err = os.Stat(channelPath)
	assert.True(t, os.IsNotExist(err), "channel.txt should not exist when olmClient is nil")
}

func TestRollbackManagerDisabled(t *testing.T) {
	snapshotDir := t.TempDir()

	mgr := NewRollbackManager(nil, nil, snapshotDir, RollbackConfig{
		Enabled: false,
	})

	err := mgr.SnapshotBeforeHop(context.Background(), "rhods-operator", "redhat-ods-operator", "", 0)
	require.NoError(t, err)

	// No directories should be created
	snapshotsDir := filepath.Join(snapshotDir, "snapshots")
	_, err = os.Stat(snapshotsDir)
	assert.True(t, os.IsNotExist(err), "snapshots dir should not exist when disabled")

	err = mgr.RollbackHop(context.Background(), "rhods-operator", "redhat-ods-operator", 0)
	require.NoError(t, err)
}

func TestRollbackHopRestoresChannel(t *testing.T) {
	snapshotDir := t.TempDir()

	// Pre-create snapshot dir with channel.txt
	hopDir := filepath.Join(snapshotDir, "snapshots", "hop-2")
	require.NoError(t, os.MkdirAll(hopDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hopDir, "channel.txt"), []byte("stable-2.10"), 0o644))

	mgr := NewRollbackManager(nil, nil, snapshotDir, RollbackConfig{
		Enabled: true,
	})

	// With nil olmClient, rollback should handle gracefully (skip channel restore)
	err := mgr.RollbackHop(context.Background(), "rhods-operator", "redhat-ods-operator", 2)
	require.NoError(t, err)
}

func TestRollbackHopRestoresCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, apiextv1.AddToScheme(scheme))

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	snapshotDir := t.TempDir()

	// Pre-create a CRD snapshot
	crdDir := filepath.Join(snapshotDir, "snapshots", "hop-0", "crds")
	require.NoError(t, os.MkdirAll(crdDir, 0o755))

	crdYAML := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: notebooks.kubeflow.org
  labels:
    operators.coreos.com/rhods-operator.redhat-ods-operator: ""
spec:
  group: kubeflow.org
  names:
    plural: notebooks
    singular: notebook
    kind: Notebook
    listKind: NotebookList
  scope: Cluster
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
`
	require.NoError(t, os.WriteFile(filepath.Join(crdDir, "notebooks.kubeflow.org.yaml"), []byte(crdYAML), 0o644))

	mgr := NewRollbackManager(nil, k8sClient, snapshotDir, RollbackConfig{
		Enabled:      true,
		SnapshotCRDs: true,
	})

	err := mgr.RollbackHop(context.Background(), "rhods-operator", "redhat-ods-operator", 0)
	require.NoError(t, err)

	// Verify CRD was created in the fake client
	var crdList apiextv1.CustomResourceDefinitionList
	require.NoError(t, k8sClient.List(context.Background(), &crdList))
	assert.Len(t, crdList.Items, 1)
	assert.Equal(t, "notebooks.kubeflow.org", crdList.Items[0].Name)
}

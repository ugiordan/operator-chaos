package olm

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestClient(t *testing.T, objects ...client.Object) *Client {
	t.Helper()
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "SubscriptionList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersion"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersionList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "InstallPlan"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "InstallPlanList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "packages.operators.coreos.com", Version: "v1", Kind: "PackageManifest"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "packages.operators.coreos.com", Version: "v1", Kind: "PackageManifestList"},
		&unstructured.UnstructuredList{},
	)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objects {
		builder = builder.WithObjects(obj)
	}
	k8s := builder.Build()
	return NewClient(k8s, log.New(os.Stderr, "[olm-test] ", 0))
}

func makeSubscription(name, namespace, channel, installedCSV, currentCSV string) *unstructured.Unstructured {
	sub := &unstructured.Unstructured{}
	sub.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription",
	})
	sub.SetName(name)
	sub.SetNamespace(namespace)
	sub.Object["spec"] = map[string]interface{}{
		"channel": channel,
	}
	status := map[string]interface{}{}
	if installedCSV != "" {
		status["installedCSV"] = installedCSV
	}
	if currentCSV != "" {
		status["currentCSV"] = currentCSV
	}
	sub.Object["status"] = status
	return sub
}

func makePackageManifest(name, namespace string, channels []ChannelInfo) *unstructured.Unstructured {
	pm := &unstructured.Unstructured{}
	pm.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "packages.operators.coreos.com", Version: "v1", Kind: "PackageManifest",
	})
	pm.SetName(name)
	pm.SetNamespace(namespace)
	var chList []interface{}
	for _, ch := range channels {
		chList = append(chList, map[string]interface{}{
			"name":       ch.Name,
			"currentCSV": ch.CSVName,
			"currentCSVDesc": map[string]interface{}{
				"version": ch.HeadVersion,
			},
		})
	}
	pm.Object["status"] = map[string]interface{}{
		"channels": chList,
	}
	return pm
}

func TestDiscover(t *testing.T) {
	pm := makePackageManifest("rhods-operator", "redhat-ods-operator", []ChannelInfo{
		{Name: "stable-2.10", HeadVersion: "2.10.1", CSVName: "rhods-operator.v2.10.1"},
		{Name: "stable-3.3", HeadVersion: "3.3.1", CSVName: "rhods-operator.v3.3.1"},
	})
	c := newTestClient(t, pm)

	channels, err := c.Discover(context.Background(), "rhods-operator", "redhat-ods-operator")
	require.NoError(t, err)
	require.Len(t, channels, 2)
	assert.Equal(t, "stable-2.10", channels[0].Name)
	assert.Equal(t, "stable-3.3", channels[1].Name)
}

func TestGetCurrentVersion(t *testing.T) {
	sub := makeSubscription("rhods-operator", "redhat-ods-operator", "stable-2.10",
		"rhods-operator.v2.10.1", "rhods-operator.v2.10.1")
	c := newTestClient(t, sub)

	version, err := c.GetCurrentVersion(context.Background(), "rhods-operator", "redhat-ods-operator")
	require.NoError(t, err)
	assert.Equal(t, "rhods-operator.v2.10.1", version)
}

func TestPatchChannel(t *testing.T) {
	sub := makeSubscription("rhods-operator", "redhat-ods-operator", "stable-2.10",
		"rhods-operator.v2.10.1", "rhods-operator.v2.10.1")
	c := newTestClient(t, sub)

	err := c.PatchChannel(context.Background(), "rhods-operator", "redhat-ods-operator", "stable-3.3")
	require.NoError(t, err)

	// Verify the subscription was patched
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "Subscription",
	})
	err = c.k8s.Get(context.Background(), client.ObjectKey{
		Name: "rhods-operator", Namespace: "redhat-ods-operator",
	}, got)
	require.NoError(t, err)
	channel, err := getSubscriptionChannel(got)
	require.NoError(t, err)
	assert.Equal(t, "stable-3.3", channel)
}

func TestDiscoverNotFound(t *testing.T) {
	c := newTestClient(t)

	_, err := c.Discover(context.Background(), "nonexistent", "ns")
	assert.Error(t, err)
}

func TestGetCurrentVersionNoSubscription(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetCurrentVersion(context.Background(), "nonexistent", "ns")
	assert.Error(t, err)
}

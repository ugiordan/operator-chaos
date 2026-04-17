package olm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestWatchUpgradeSucceeded(t *testing.T) {
	// Create subscription with a new currentCSV (simulating post-patch state)
	sub := makeSubscription("rhods-operator", "redhat-ods-operator", "stable-3.3",
		"rhods-operator.v2.10.1", "rhods-operator.v3.3.1")
	// Add installPlanRef to subscription status
	sub.Object["status"].(map[string]interface{})["installPlanRef"] = map[string]interface{}{
		"name": "install-abc", "namespace": "redhat-ods-operator",
	}

	// Create install plan (approved, complete)
	ip := &unstructured.Unstructured{}
	ip.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "InstallPlan",
	})
	ip.SetName("install-abc")
	ip.SetNamespace("redhat-ods-operator")
	ip.Object["spec"] = map[string]interface{}{"approval": "Automatic", "approved": true}
	ip.Object["status"] = map[string]interface{}{"phase": "Complete"}

	// Create CSV (succeeded)
	csv := &unstructured.Unstructured{}
	csv.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "ClusterServiceVersion",
	})
	csv.SetName("rhods-operator.v3.3.1")
	csv.SetNamespace("redhat-ods-operator")
	csv.Object["status"] = map[string]interface{}{"phase": "Succeeded"}

	c := newTestClient(t, sub, ip, csv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	statusCh, err := c.WatchUpgrade(ctx, "rhods-operator", "redhat-ods-operator", 100*time.Millisecond)
	require.NoError(t, err)

	var statuses []UpgradeStatus
	for s := range statusCh {
		statuses = append(statuses, s)
	}

	require.NotEmpty(t, statuses)
	last := statuses[len(statuses)-1]
	assert.Equal(t, PhaseSucceeded, last.Phase)
}

func TestWatchUpgradeContextCancelled(t *testing.T) {
	sub := makeSubscription("rhods-operator", "redhat-ods-operator", "stable-3.3",
		"rhods-operator.v2.10.1", "rhods-operator.v2.10.1") // currentCSV hasn't changed yet

	c := newTestClient(t, sub)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	statusCh, err := c.WatchUpgrade(ctx, "rhods-operator", "redhat-ods-operator", 50*time.Millisecond)
	require.NoError(t, err)

	var statuses []UpgradeStatus
	for s := range statusCh {
		statuses = append(statuses, s)
	}

	// Should have gotten at least a Pending status before context cancelled
	if len(statuses) > 0 {
		last := statuses[len(statuses)-1]
		assert.NotEqual(t, PhaseSucceeded, last.Phase)
	}
}

func TestWatchUpgradeManualApproval(t *testing.T) {
	sub := makeSubscription("rhods-operator", "redhat-ods-operator", "stable-3.3",
		"rhods-operator.v2.10.1", "rhods-operator.v3.3.1")
	sub.Object["status"].(map[string]interface{})["installPlanRef"] = map[string]interface{}{
		"name": "install-manual", "namespace": "redhat-ods-operator",
	}

	ip := &unstructured.Unstructured{}
	ip.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v1alpha1", Kind: "InstallPlan",
	})
	ip.SetName("install-manual")
	ip.SetNamespace("redhat-ods-operator")
	ip.Object["spec"] = map[string]interface{}{"approval": "Manual", "approved": false}
	ip.Object["status"] = map[string]interface{}{"phase": "RequiresApproval"}

	c := newTestClient(t, sub, ip)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	statusCh, err := c.WatchUpgrade(ctx, "rhods-operator", "redhat-ods-operator", 100*time.Millisecond)
	require.NoError(t, err)

	var foundManualMsg bool
	for s := range statusCh {
		if s.InstallPlanApproval == "Manual" {
			foundManualMsg = true
		}
	}
	assert.True(t, foundManualMsg, "expected a status message about manual approval")
}

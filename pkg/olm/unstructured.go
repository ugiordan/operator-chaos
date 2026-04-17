package olm

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	subscriptionGVR = schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions",
	}
	csvGVR = schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions",
	}
	installPlanGVR = schema.GroupVersionResource{
		Group: "operators.coreos.com", Version: "v1alpha1", Resource: "installplans",
	}
	packageManifestGVR = schema.GroupVersionResource{
		Group: "packages.operators.coreos.com", Version: "v1", Resource: "packagemanifests",
	}
)

// getSubscriptionStatus extracts installedCSV and currentCSV from a Subscription's status.
func getSubscriptionStatus(obj *unstructured.Unstructured) (installedCSV, currentCSV string, err error) {
	status, ok := obj.Object["status"].(map[string]interface{})
	if !ok {
		return "", "", nil
	}
	if v, ok := status["installedCSV"].(string); ok {
		installedCSV = v
	}
	if v, ok := status["currentCSV"].(string); ok {
		currentCSV = v
	}
	return installedCSV, currentCSV, nil
}

// getSubscriptionInstallPlanRef extracts the InstallPlan name and namespace from a Subscription's status.
func getSubscriptionInstallPlanRef(obj *unstructured.Unstructured) (name, namespace string, err error) {
	ref, found, err := unstructured.NestedMap(obj.Object, "status", "installPlanRef")
	if err != nil || !found {
		return "", "", err
	}
	if v, ok := ref["name"].(string); ok {
		name = v
	}
	if v, ok := ref["namespace"].(string); ok {
		namespace = v
	}
	return name, namespace, nil
}

// getCSVPhase extracts the phase from a ClusterServiceVersion's status.
func getCSVPhase(obj *unstructured.Unstructured) (string, error) {
	phase, found, err := unstructured.NestedString(obj.Object, "status", "phase")
	if err != nil || !found {
		return "", err
	}
	return phase, nil
}

// getInstallPlanPhase extracts the phase from an InstallPlan's status.
func getInstallPlanPhase(obj *unstructured.Unstructured) (string, error) {
	phase, found, err := unstructured.NestedString(obj.Object, "status", "phase")
	if err != nil || !found {
		return "", err
	}
	return phase, nil
}

// getInstallPlanApproval extracts the approval strategy from an InstallPlan's spec.
func getInstallPlanApproval(obj *unstructured.Unstructured) (string, error) {
	approval, found, err := unstructured.NestedString(obj.Object, "spec", "approval")
	if err != nil || !found {
		return "", err
	}
	return approval, nil
}

// getPackageChannels extracts channel info from a PackageManifest's status.
func getPackageChannels(obj *unstructured.Unstructured) ([]ChannelInfo, error) {
	channels, found, err := unstructured.NestedSlice(obj.Object, "status", "channels")
	if err != nil || !found {
		return nil, err
	}

	var result []ChannelInfo
	for _, ch := range channels {
		chMap, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		info := ChannelInfo{}
		if v, ok := chMap["name"].(string); ok {
			info.Name = v
		}
		if v, ok := chMap["currentCSV"].(string); ok {
			info.CSVName = v
		}
		if desc, ok := chMap["currentCSVDesc"].(map[string]interface{}); ok {
			if v, ok := desc["version"].(string); ok {
				info.HeadVersion = v
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// getSubscriptionChannel extracts the spec.channel from a Subscription.
func getSubscriptionChannel(obj *unstructured.Unstructured) (string, error) {
	channel, found, err := unstructured.NestedString(obj.Object, "spec", "channel")
	if err != nil || !found {
		return "", fmt.Errorf("subscription spec.channel not found")
	}
	return channel, nil
}

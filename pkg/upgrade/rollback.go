package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-chaos/pkg/olm"
)

// RollbackManager handles snapshot and restore of operator state around upgrade hops.
type RollbackManager struct {
	olmClient   *olm.Client
	k8sClient   client.Client
	snapshotDir string
	config      RollbackConfig
}

// NewRollbackManager creates a new RollbackManager.
func NewRollbackManager(olmClient *olm.Client, k8sClient client.Client, snapshotDir string, config RollbackConfig) *RollbackManager {
	return &RollbackManager{
		olmClient:   olmClient,
		k8sClient:   k8sClient,
		snapshotDir: snapshotDir,
		config:      config,
	}
}

// hopDir returns the snapshot directory for a given hop index.
func (r *RollbackManager) hopDir(hopIndex int) string {
	return filepath.Join(r.snapshotDir, "snapshots", fmt.Sprintf("hop-%d", hopIndex))
}

// SnapshotBeforeHop captures operator state before an upgrade hop.
// currentChannel is the channel the subscription is on before this hop.
// The caller (OLMExecutor) knows this from the playbook's hop list.
// When rollback is disabled, this is a no-op.
func (r *RollbackManager) SnapshotBeforeHop(ctx context.Context, operator, namespace, currentChannel string, hopIndex int) error {
	if !r.config.Enabled {
		return nil
	}

	hopPath := r.hopDir(hopIndex)
	crdPath := filepath.Join(hopPath, "crds")

	if err := os.MkdirAll(crdPath, 0o755); err != nil {
		return fmt.Errorf("creating snapshot dir %s: %w", crdPath, err)
	}

	// Save the current channel so rollback can restore it
	if currentChannel != "" {
		channelFile := filepath.Join(hopPath, "channel.txt")
		if err := os.WriteFile(channelFile, []byte(currentChannel), 0o644); err != nil {
			return fmt.Errorf("writing channel.txt: %w", err)
		}
	}

	// Snapshot CRDs owned by the operator
	if r.config.SnapshotCRDs && r.k8sClient != nil {
		var crdList apiextv1.CustomResourceDefinitionList
		labelKey := fmt.Sprintf("operators.coreos.com/%s.%s", operator, namespace)

		if err := r.k8sClient.List(ctx, &crdList, client.HasLabels{labelKey}); err != nil {
			return fmt.Errorf("listing CRDs for %s: %w", labelKey, err)
		}

		for i := range crdList.Items {
			crd := &crdList.Items[i]
			data, err := sigsyaml.Marshal(crd)
			if err != nil {
				return fmt.Errorf("marshaling CRD %s: %w", crd.Name, err)
			}
			filename := filepath.Join(crdPath, crd.Name+".yaml")
			if err := os.WriteFile(filename, data, 0o644); err != nil {
				return fmt.Errorf("writing CRD snapshot %s: %w", filename, err)
			}
		}
	}

	return nil
}

// RollbackHop restores operator state from a previous snapshot.
// When rollback is disabled, this is a no-op.
func (r *RollbackManager) RollbackHop(ctx context.Context, operator, namespace string, hopIndex int) error {
	if !r.config.Enabled {
		return nil
	}

	hopPath := r.hopDir(hopIndex)

	// Restore channel from snapshot
	if r.olmClient != nil {
		channelFile := filepath.Join(hopPath, "channel.txt")
		data, err := os.ReadFile(channelFile)
		if err == nil && len(data) > 0 {
			channel := strings.TrimSpace(string(data))
			if err := r.olmClient.PatchChannel(ctx, operator, namespace, channel); err != nil {
				return fmt.Errorf("restoring channel to %s: %w", channel, err)
			}
		}
		// If channel.txt doesn't exist, skip gracefully
	}

	// Restore CRDs from snapshot
	if r.config.SnapshotCRDs && r.k8sClient != nil {
		crdPath := filepath.Join(hopPath, "crds")
		entries, err := os.ReadDir(crdPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("reading CRD snapshot dir: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(crdPath, entry.Name()))
			if err != nil {
				return fmt.Errorf("reading CRD snapshot %s: %w", entry.Name(), err)
			}

			obj := &unstructured.Unstructured{}
			if err := sigsyaml.Unmarshal(data, &obj.Object); err != nil {
				return fmt.Errorf("unmarshaling CRD %s: %w", entry.Name(), err)
			}

			// Set GVK explicitly for unstructured objects
			if obj.GetAPIVersion() == "" {
				obj.SetGroupVersionKind(apiextv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
			}

			// Try to get the existing object so we can copy its resourceVersion for Update.
			// If it doesn't exist, fall back to Create.
			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(obj.GroupVersionKind())
			getErr := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), existing)
			if getErr == nil {
				obj.SetResourceVersion(existing.GetResourceVersion())
				if err := r.k8sClient.Update(ctx, obj); err != nil {
					return fmt.Errorf("restoring CRD %s: %w", entry.Name(), err)
				}
			} else {
				obj.SetResourceVersion("")
				if err := r.k8sClient.Create(ctx, obj); err != nil {
					return fmt.Errorf("restoring CRD %s: %w", entry.Name(), err)
				}
			}
		}
	}

	return nil
}


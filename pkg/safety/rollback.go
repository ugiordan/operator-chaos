package safety

const (
	// RollbackAnnotationKey is the annotation key used to store original resource
	// state for rollback during chaos cleanup.
	RollbackAnnotationKey = "chaos.opendatahub.io/rollback-data"

	// ManagedByLabel is the standard Kubernetes label for tracking resource ownership.
	ManagedByLabel = "app.kubernetes.io/managed-by"

	// ManagedByValue is the value used in managed-by labels for chaos resources.
	ManagedByValue = "odh-chaos"

	// ChaosTypeLabel is the label key used to identify the type of chaos injection
	// that created a resource.
	ChaosTypeLabel = "chaos.opendatahub.io/type"
)

// ChaosLabels returns standard labels for chaos-managed resources.
func ChaosLabels(injectionType string) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		ChaosTypeLabel: injectionType,
	}
}

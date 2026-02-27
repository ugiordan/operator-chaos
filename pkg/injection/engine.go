package injection

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
)

type CleanupFunc func(ctx context.Context) error

type Injector interface {
	Validate(spec v1alpha1.InjectionSpec, blast v1alpha1.BlastRadiusSpec) error
	Inject(ctx context.Context, spec v1alpha1.InjectionSpec, namespace string) (CleanupFunc, []v1alpha1.InjectionEvent, error)
}

type Registry struct {
	injectors map[v1alpha1.InjectionType]Injector
}

func NewRegistry() *Registry {
	return &Registry{
		injectors: make(map[v1alpha1.InjectionType]Injector),
	}
}

func (r *Registry) Register(t v1alpha1.InjectionType, i Injector) {
	r.injectors[t] = i
}

func (r *Registry) Get(t v1alpha1.InjectionType) (Injector, error) {
	if inj, ok := r.injectors[t]; ok {
		return inj, nil
	}
	return nil, fmt.Errorf("unknown injection type %q; registered types: %v", t, r.ListTypes())
}

func (r *Registry) ListTypes() []v1alpha1.InjectionType {
	types := make([]v1alpha1.InjectionType, 0, len(r.injectors))
	for t := range r.injectors {
		types = append(types, t)
	}
	return types
}

func NewEvent(t v1alpha1.InjectionType, target string, action string, details map[string]string) v1alpha1.InjectionEvent {
	return v1alpha1.InjectionEvent{
		Timestamp: time.Now(),
		Type:      t,
		Target:    target,
		Action:    action,
		Details:   details,
	}
}

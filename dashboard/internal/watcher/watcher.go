package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/convert"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
)

type Upserter interface {
	Upsert(exp store.Experiment) error
}

type Broadcaster interface {
	Broadcast(data []byte)
}

type Watcher struct {
	client      client.Client
	store       Upserter
	broadcaster Broadcaster
}

func NewWatcher(c client.Client, s Upserter, b Broadcaster) *Watcher {
	return &Watcher{client: c, store: s, broadcaster: b}
}

func (w *Watcher) SyncOnce(ctx context.Context) error {
	var list v1alpha1.ChaosExperimentList
	if err := w.client.List(ctx, &list); err != nil {
		return fmt.Errorf("listing ChaosExperiments: %w", err)
	}

	for i := range list.Items {
		if err := w.handleCREvent(&list.Items[i]); err != nil {
			log.Printf("error processing %s/%s: %v", list.Items[i].Namespace, list.Items[i].Name, err)
		}
	}
	return nil
}

func (w *Watcher) handleCREvent(cr *v1alpha1.ChaosExperiment) error {
	exp, err := convert.FromCR(cr)
	if err != nil {
		return fmt.Errorf("converting CR %s/%s: %w", cr.Namespace, cr.Name, err)
	}
	if err := w.store.Upsert(*exp); err != nil {
		return err
	}

	if w.broadcaster != nil {
		data, _ := json.Marshal(exp)
		w.broadcaster.Broadcast(data)
	}
	return nil
}

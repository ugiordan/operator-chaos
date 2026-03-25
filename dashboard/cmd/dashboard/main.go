package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-platform-chaos/api/v1alpha1"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/api"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/store"
	"github.com/opendatahub-io/odh-platform-chaos/dashboard/internal/watcher"
	"github.com/opendatahub-io/odh-platform-chaos/pkg/model"
)

func main() {
	var (
		addr         = flag.String("addr", ":8080", "HTTP listen address")
		dbPath       = flag.String("db", "dashboard.db", "SQLite database path")
		kubeconfig   = flag.String("kubeconfig", "", "Path to kubeconfig (uses in-cluster if empty)")
		syncInterval = flag.Duration("sync-interval", 30*time.Second, "Interval for K8s sync")
	)
	flag.Parse()

	s, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		log.Fatalf("opening store: %v", err)
	}
	defer s.Close()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		log.Fatalf("adding scheme: %v", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("building kubeconfig: %v", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("creating k8s client: %v", err)
	}

	var knowledge []model.OperatorKnowledge

	broker := api.NewSSEBroker()
	go broker.Run()

	w := watcher.NewWatcher(k8sClient, s, broker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.SyncOnce(ctx); err != nil {
		log.Printf("warning: initial sync failed: %v", err)
	}

	go func() {
		ticker := time.NewTicker(*syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := w.SyncOnce(ctx); err != nil {
					log.Printf("sync error: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	srv := api.NewServer(s, broker, knowledge)
	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()
		broker.Stop()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("dashboard listening on %s", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	fmt.Println("dashboard stopped")
}

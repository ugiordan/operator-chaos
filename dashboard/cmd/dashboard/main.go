package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
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
	dashboard "github.com/opendatahub-io/odh-platform-chaos/dashboard"
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
		knowledgeDir = flag.String("knowledge-dir", "", "Path to directory containing operator knowledge YAML files")
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
	if *knowledgeDir != "" {
		loaded, err := model.LoadKnowledgeDir(*knowledgeDir)
		if err != nil {
			log.Fatalf("loading knowledge: %v", err)
		}
		for _, k := range loaded {
			knowledge = append(knowledge, *k)
		}
		log.Printf("loaded %d operator knowledge files from %s", len(knowledge), *knowledgeDir)
	}

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

	// Serve embedded UI assets
	uiFS, err := fs.Sub(dashboard.UIAssets, "ui-dist")
	if err != nil {
		log.Fatalf("embedded ui assets: %v", err)
	}
	fileServer := http.FileServer(http.FS(uiFS))

	// Combine API + static file serving
	mux := http.NewServeMux()
	mux.Handle("/api/", srv.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file; fall back to index.html for SPA routing
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		if _, err := fs.Stat(uiFS, path[1:]); err != nil {
			// File not found — serve index.html for client-side routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
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

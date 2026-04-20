# Development Setup

This guide walks you through setting up a local development environment for Operator Chaos.

## Prerequisites

### Required Tools

- **Go 1.25 or later** — [Install Go](https://go.dev/doc/install)
- **Git** — For cloning the repository
- **kubectl** — [Install kubectl](https://kubernetes.io/docs/tasks/tools/)
- **Access to a Kubernetes cluster** — Kind, Minikube, or OpenShift

### Optional Tools

- **kind** — [Install kind](https://kind.sigs.k8s.io/docs/user/quick-start/) (recommended for local testing)
- **golangci-lint** — [Install golangci-lint](https://golangci-lint.run/welcome/install/) (for linting)
- **make** — For using Makefile targets

## Clone the Repository

```bash
git clone https://github.com/ugiordan/odh-platform-chaos.git
cd odh-platform-chaos
```

## Verify Go Version

```bash
go version
# Should output: go version go1.25.x ...
```

If your Go version is older, update it before proceeding.

## Install Dependencies

```bash
go mod download
```

This downloads all Go module dependencies defined in `go.mod`.

## Build the Project

### Build All Binaries

```bash
go build ./...
```

This compiles all packages and ensures there are no syntax or type errors.

### Build the CLI

```bash
go build -o bin/chaos-cli ./cmd/cli
```

The `chaos-cli` binary will be placed in `bin/chaos-cli`.

### Build the Controller

```bash
go build -o bin/chaos-controller ./cmd/controller
```

## Run Tests

### Unit Tests

```bash
go test ./...
```

Run tests with verbose output:

```bash
go test -v ./...
```

### Test Specific Packages

```bash
go test ./pkg/injection/...
go test ./pkg/observer/...
```

### Run with Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

Open `coverage.html` in a browser to view coverage reports.

## Project Structure

```
odh-platform-chaos/
├── api/v1alpha1/              # CRD types and validation
│   ├── types.go               # ChaosExperiment CRD definition
│   └── groupversion_info.go   # API group metadata
├── cmd/
│   ├── cli/                   # CLI entrypoint
│   └── controller/            # Controller entrypoint
├── pkg/
│   ├── injection/             # Injection engine
│   │   ├── engine.go          # Registry and Injector interface
│   │   ├── podkill.go         # PodKill implementation
│   │   ├── network.go         # NetworkPartition implementation
│   │   └── ...                # Other injectors
│   ├── observer/              # Observation system
│   │   ├── board.go           # Blackboard implementation
│   │   ├── contributor.go     # Contributor interface
│   │   └── ...                # Specific contributors
│   ├── orchestrator/          # Experiment orchestration
│   │   └── lifecycle.go       # Lifecycle state machine
│   ├── evaluator/             # Verdict computation
│   ├── reporter/              # Report generation
│   ├── safety/                # Blast radius and safety checks
│   ├── model/                 # Operator knowledge and dependency graph
│   └── sdk/                   # Go SDK for client-side chaos
│       ├── client.go          # ChaosClient wrapper
│       ├── types.go           # FaultConfig types
│       └── faults/            # Fault injection primitives
├── config/
│   ├── crd/                   # CRD manifests
│   ├── controller/            # Controller deployment manifests
│   └── samples/               # Example experiments
├── experiments/               # Additional experiment examples
├── site/                      # Documentation (MkDocs)
└── Makefile                   # Build automation
```

## Running the CLI Locally

### Run an Experiment

```bash
./bin/chaos-cli run experiments/podkill-basic.yaml
```

### Validate an Experiment

```bash
./bin/chaos-cli validate experiments/podkill-basic.yaml
```

### List Available Injection Types

```bash
./bin/chaos-cli list-types
```

### Generate Report

```bash
./bin/chaos-cli run experiments/podkill-basic.yaml --report-dir=./reports
```

Reports are saved as JSON files in the specified directory.

## Running the Controller Locally

### 1. Set Up a Local Cluster

#### Using kind

```bash
kind create cluster --name chaos-test
```

#### Using Minikube

```bash
minikube start --driver=docker
```

### 2. Install CRDs

```bash
kubectl apply -f config/crd/
```

Verify CRD installation:

```bash
kubectl get crd chaosexperiments.chaos.operatorchaos.io
```

### 3. Run Controller Locally

```bash
export KUBECONFIG=~/.kube/config
./bin/chaos-controller
```

The controller will watch for `ChaosExperiment` resources and reconcile them.

**Controller Logs:**

```
INFO    controller-runtime.metrics    Metrics server is starting to listen
INFO    controller-runtime.builder    Starting EventSource
INFO    controller-runtime.builder    Starting Controller
INFO    controller-runtime.controller Starting workers
```

### 4. Submit an Experiment

In another terminal:

```bash
kubectl apply -f experiments/podkill-basic.yaml
```

Watch experiment progress:

```bash
kubectl get chaosexperiment podkill-basic -w
```

View experiment status:

```bash
kubectl describe chaosexperiment podkill-basic
```

## Running the Dashboard Locally

The dashboard is a web UI for viewing experiment results.

### 1. Install Node.js Dependencies

```bash
cd dashboard
npm install
```

### 2. Start Development Server

```bash
npm run dev
```

The dashboard will be available at `http://localhost:3000`.

### 3. Configure API Endpoint

Edit `dashboard/.env.local`:

```
NEXT_PUBLIC_API_URL=http://localhost:8080
```

### 4. Run API Server (Optional)

If the controller is running, it exposes an API server on port 8080 by default.

## Code Quality Tools

### Linting

```bash
golangci-lint run
```

Fix auto-fixable issues:

```bash
golangci-lint run --fix
```

### Formatting

```bash
go fmt ./...
```

### Vet

```bash
go vet ./...
```

## Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/my-new-feature
```

### 2. Make Changes

Edit code, add tests, update documentation.

### 3. Run Tests

```bash
go test ./...
```

### 4. Lint and Format

```bash
golangci-lint run
go fmt ./...
```

### 5. Commit Changes

```bash
git add .
git commit -m "feat: add new injection type"
```

### 6. Push and Open PR

```bash
git push origin feature/my-new-feature
```

Open a pull request on GitHub.

## Testing Against a Real Cluster

### 1. Deploy Target Operators

Deploy the operators you want to test. For example, to test with OpenDataHub, follow the [ODH installation guide](https://opendatahub.io/docs/getting-started/quick-installation.html).

### 2. Apply Operator Knowledge

```bash
kubectl apply -f config/knowledge/opendatahub-operators.yaml
```

### 3. Run Experiments

```bash
kubectl apply -f experiments/odh-controller-resilience.yaml
```

### 4. View Results

```bash
kubectl get chaosexperiment -A
kubectl get configmap -l app.kubernetes.io/managed-by=operator-chaos
```

## Debugging

### Enable Debug Logging

Set `CHAOS_LOG_LEVEL=debug` when running the controller or CLI:

```bash
export CHAOS_LOG_LEVEL=debug
./bin/chaos-controller
```

### Inspect Chaos-Managed Resources

List all resources managed by chaos:

```bash
kubectl get all -A -l chaos.operatorchaos.io/managed=true
```

### View Rollback Annotations

```bash
kubectl get networkpolicy operator-chaos-np-app-redis -o yaml | grep -A 5 annotations
```

### Controller Restart Recovery

To test crash-safe cleanup, kill the controller mid-experiment and restart it:

```bash
# Kill controller
pkill chaos-controller

# Restart
./bin/chaos-controller
```

The controller should detect in-progress experiments and clean them up via `Revert()`.

## Common Issues

### "CRD not found"

**Solution:** Install CRDs:

```bash
kubectl apply -f config/crd/
```

### "Permission denied" errors from controller

**Solution:** Apply RBAC manifests:

```bash
kubectl apply -f config/controller/rbac.yaml
```

### Experiments stuck in "Pending"

**Solution:** Check controller logs for validation errors:

```bash
kubectl logs -n opendatahub deploy/chaos-controller
```

### TTL cleanup not working

**Solution:** Ensure the cleanup controller is running:

```bash
kubectl get pod -n opendatahub -l app=chaos-cleanup-controller
```

## Next Steps

- [Adding Injection Types](adding-injection-types.md) — Implement a new injector
- [Architecture Overview](../architecture/overview.md) — Understand system design
- [Go SDK Reference](../reference/go-sdk.md) — Use SDK in your operators

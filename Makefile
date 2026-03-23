BINARY := odh-chaos
PKG := github.com/opendatahub-io/odh-platform-chaos
CMD := ./cmd/odh-chaos

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X $(PKG)/internal/cli.Version=$(VERSION)

# Container image settings
CONTAINER_TOOL ?= podman
IMAGE_REGISTRY ?= quay.io/opendatahub
IMAGE_NAME ?= odh-chaos
IMAGE_TAG ?= $(VERSION)
IMAGE ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# Code generation tools
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen

.PHONY: build test test-short lint clean install container-build container-push generate manifests

generate: ## Generate deepcopy methods
	$(CONTROLLER_GEN) object paths=./api/...

manifests: ## Generate CRD manifests
	$(CONTROLLER_GEN) crd paths=./api/... output:crd:dir=config/crd/bases

build: generate
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

test: generate
	go test -race ./... -v -count=1

test-short:
	go test ./... -short -count=1

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/

container-build:
	$(CONTAINER_TOOL) build --build-arg VERSION=$(VERSION) --build-arg TARGETARCH=$(shell go env GOARCH) -t $(IMAGE) -f Containerfile .

container-push:
	$(CONTAINER_TOOL) push $(IMAGE)

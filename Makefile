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

.PHONY: build test test-short lint clean install container-build container-push

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

test:
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

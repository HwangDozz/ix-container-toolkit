IMAGE_REGISTRY ?= docker.io/accelerator-toolkit
IMAGE_TAG      ?= latest
DRA_IMAGE      := $(IMAGE_REGISTRY)/dra-driver:$(IMAGE_TAG)
PROFILE        ?= profiles/iluvatar-bi-v150.yaml
MULTIARCH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_BUILD_ARGS ?=

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
LDFLAGS := -s -w
BINARY_PLATFORM_DIR := bin/$(GOOS)-$(GOARCH)

.PHONY: all build test docker-build docker-push docker-build-multiarch docker-push-multiarch deploy undeploy clean

all: build

## Build all binaries for the current GOOS/GOARCH
build:
	mkdir -p bin $(BINARY_PLATFORM_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-dra-driver ./cmd/accelerator-dra-driver
	cp $(BINARY_PLATFORM_DIR)/accelerator-dra-driver bin/accelerator-dra-driver
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-profile-render ./cmd/accelerator-profile-render
	cp $(BINARY_PLATFORM_DIR)/accelerator-profile-render bin/accelerator-profile-render

## Run unit tests
test:
	go test ./...

## Build the DRA driver container image
docker-build:
	docker build $(DOCKER_BUILD_ARGS) -t $(DRA_IMAGE) .

## Push the DRA driver image to the registry
docker-push: docker-build
	docker push $(DRA_IMAGE)

## Build and push a multi-arch DRA driver image
docker-build-multiarch:
	docker buildx build $(DOCKER_BUILD_ARGS) --platform $(MULTIARCH_PLATFORMS) -t $(DRA_IMAGE) --push .

docker-push-multiarch: docker-build-multiarch

## Deploy DRA driver to the current Kubernetes cluster
deploy:
	kubectl apply -f deployments/dra-driver/rbac.yaml
	kubectl apply -f deployments/dra-driver/daemonset.yaml

## Remove DRA driver from the cluster
undeploy:
	kubectl delete -f deployments/dra-driver/daemonset.yaml --ignore-not-found
	kubectl delete -f deployments/dra-driver/rbac.yaml --ignore-not-found

clean:
	rm -rf bin/

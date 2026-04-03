IMAGE_REGISTRY ?= docker.io/accelerator-toolkit
IMAGE_TAG      ?= latest
IMAGE          := $(IMAGE_REGISTRY)/installer:$(IMAGE_TAG)
PROFILE        ?= profiles/iluvatar-bi-v150.yaml
MULTIARCH_PLATFORMS ?= linux/amd64,linux/arm64
DOCKER_BUILD_ARGS ?=

GOOS   ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
LDFLAGS := -s -w
BINARY_PLATFORM_DIR := bin/$(GOOS)-$(GOARCH)

.PHONY: all build test docker-build docker-push docker-build-prebuilt docker-push-prebuilt docker-build-multiarch docker-push-multiarch docker-build-prebuilt-multiarch docker-push-prebuilt-multiarch deploy undeploy clean render-runtimeclass render-daemonset render-bundle

all: build

## Build all binaries for the current GOOS/GOARCH and mirror them into bin/<os>-<arch>/
build:
	mkdir -p bin $(BINARY_PLATFORM_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-container-runtime ./cmd/accelerator-container-runtime
	cp $(BINARY_PLATFORM_DIR)/accelerator-container-runtime bin/accelerator-container-runtime
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-container-hook ./cmd/accelerator-container-hook
	cp $(BINARY_PLATFORM_DIR)/accelerator-container-hook bin/accelerator-container-hook
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-installer ./cmd/accelerator-installer
	cp $(BINARY_PLATFORM_DIR)/accelerator-installer bin/accelerator-installer
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o $(BINARY_PLATFORM_DIR)/accelerator-profile-render ./cmd/accelerator-profile-render
	cp $(BINARY_PLATFORM_DIR)/accelerator-profile-render bin/accelerator-profile-render

## Run unit tests
test:
	go test ./...

## Render RuntimeClass from a generic profile
render-runtimeclass:
	go run ./cmd/accelerator-profile-render runtimeclass --profile $(PROFILE)

## Render installer DaemonSet from a generic profile
render-daemonset:
	go run ./cmd/accelerator-profile-render daemonset --profile $(PROFILE) --image $(IMAGE)

## Render full deploy bundle from a generic profile
render-bundle:
	go run ./cmd/accelerator-profile-render bundle --profile $(PROFILE) --image $(IMAGE)

## Build the installer container image
docker-build:
	docker build $(DOCKER_BUILD_ARGS) -t $(IMAGE) .

## Push the installer image to the registry
docker-push: docker-build
	docker push $(IMAGE)

## Build and push a multi-arch installer image manifest for mixed-arch clusters
docker-build-multiarch:
	docker buildx build $(DOCKER_BUILD_ARGS) --platform $(MULTIARCH_PLATFORMS) -t $(IMAGE) --push .

docker-push-multiarch: docker-build-multiarch

## Build the installer image from locally prebuilt binaries in ./bin/<os>-<arch>/
docker-build-prebuilt:
	test -x $(BINARY_PLATFORM_DIR)/accelerator-container-runtime
	test -x $(BINARY_PLATFORM_DIR)/accelerator-container-hook
	test -x $(BINARY_PLATFORM_DIR)/accelerator-installer
	docker build \
	  $(DOCKER_BUILD_ARGS) \
	  --build-arg TARGETOS=$(GOOS) \
	  --build-arg TARGETARCH=$(GOARCH) \
	  -f Dockerfile.prebuilt -t $(IMAGE) .

## Push the prebuilt-binary installer image to the registry
docker-push-prebuilt: docker-build-prebuilt
	docker push $(IMAGE)

## Build and push a multi-arch installer image manifest from prebuilt binaries
docker-build-prebuilt-multiarch:
	docker buildx build $(DOCKER_BUILD_ARGS) --platform $(MULTIARCH_PLATFORMS) -f Dockerfile.prebuilt -t $(IMAGE) --push .

docker-push-prebuilt-multiarch: docker-build-prebuilt-multiarch

## Deploy accelerator-toolkit to the current Kubernetes cluster.
## Substitutes the IMAGE placeholder in daemonset.yaml at deploy time so you
## don't have to edit the YAML by hand:
##   make deploy IMAGE_REGISTRY=myregistry.example.com/accelerator-toolkit IMAGE_TAG=v1.0
deploy:
	go run ./cmd/accelerator-profile-render bundle --profile $(PROFILE) --image $(IMAGE) | kubectl apply -f -

## Remove accelerator-toolkit from the cluster
undeploy:
	go run ./cmd/accelerator-profile-render bundle --profile $(PROFILE) --image $(IMAGE) | kubectl delete -f - --ignore-not-found

clean:
	rm -rf bin/

## Install binaries to /usr/local/bin on the current host (for testing)
install-local: build
	install -m 755 bin/accelerator-container-runtime /usr/local/bin/
	install -m 755 bin/accelerator-container-hook    /usr/local/bin/
	install -m 755 bin/accelerator-profile-render    /usr/local/bin/

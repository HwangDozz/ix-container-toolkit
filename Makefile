IMAGE_REGISTRY ?= docker.io/ix-toolkit
IMAGE_TAG      ?= latest
IMAGE          := $(IMAGE_REGISTRY)/installer:$(IMAGE_TAG)

GOOS   ?= linux
GOARCH ?= amd64
LDFLAGS := -s -w

.PHONY: all build test docker-build docker-push deploy clean

all: build

## Build all binaries for Linux
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o bin/ix-container-runtime ./cmd/ix-container-runtime
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o bin/ix-container-hook ./cmd/ix-container-hook
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -trimpath -ldflags="$(LDFLAGS)" \
	  -o bin/ix-installer ./cmd/ix-installer

## Run unit tests
test:
	go test ./...

## Build the installer container image
docker-build:
	docker build -t $(IMAGE) .

## Push the installer image to the registry
docker-push: docker-build
	docker push $(IMAGE)

## Deploy ix-toolkit to the current Kubernetes cluster
deploy:
	kubectl apply -f deployments/rbac/rbac.yaml
	kubectl apply -f deployments/daemonset/daemonset.yaml

## Remove ix-toolkit from the cluster
undeploy:
	kubectl delete -f deployments/daemonset/daemonset.yaml --ignore-not-found
	kubectl delete -f deployments/rbac/rbac.yaml --ignore-not-found

clean:
	rm -rf bin/

## Install binaries to /usr/local/bin on the current host (for testing)
install-local: build
	install -m 755 bin/ix-container-runtime /usr/local/bin/
	install -m 755 bin/ix-container-hook    /usr/local/bin/

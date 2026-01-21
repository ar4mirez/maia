# Image configuration
VERSION ?= v0.1.0
IMG ?= ghcr.io/ar4mirez/maia-operator:$(VERSION)

# Get the currently used golang install path
GOBIN := $(shell go env GOPATH)/bin

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CRD manifests.
	$(CONTROLLER_GEN) rbac:roleName=maia-operator-role crd webhook paths="./..." output:crd:artifacts:config=config/crd

.PHONY: generate
generate: controller-gen ## Generate DeepCopy code.
	$(CONTROLLER_GEN) object:headerFile="" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	go test ./pkg/... -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run e2e tests (requires envtest).
	go test ./internal/controller/... -v -ginkgo.v

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build operator binary.
	go build -o bin/manager ./cmd/operator

.PHONY: run
run: manifests generate fmt vet ## Run operator locally.
	go run ./cmd/operator

.PHONY: docker-build
docker-build: ## Build docker image.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image.
	docker push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push multi-arch docker image.
	docker buildx build --platform linux/amd64,linux/arm64 -t ${IMG} --push .

##@ Deployment

.PHONY: install
install: manifests ## Install CRDs into the cluster.
	kubectl apply -f ../deployments/kubernetes/crds/

.PHONY: uninstall
uninstall: ## Uninstall CRDs from the cluster.
	kubectl delete -f ../deployments/kubernetes/crds/

.PHONY: deploy
deploy: manifests ## Deploy operator to the cluster.
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

.PHONY: undeploy
undeploy: ## Undeploy operator from the cluster.
	kubectl delete -f config/manager/
	kubectl delete -f config/rbac/

##@ Tools

CONTROLLER_GEN = $(GOBIN)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	@test -f $(CONTROLLER_GEN) || go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run

.PHONY: clean
clean: ## Clean build artifacts.
	rm -rf bin/ cover.out

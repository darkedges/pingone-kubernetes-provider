# Image and module configuration
MODULE      ?= github.com/darkedges/pingone-operator
IMG         ?= pingone-operator:latest
PLATFORM    ?= linux/amd64
HELM_CHART_DIR ?= charts/pingone-operator
HELM_PACKAGE_DIR ?= dist
HELM_OCI_REPO ?= oci://ghcr.io/darkedges/charts
HELM_VERSION ?= 0.1.0

# Directories — use absolute paths so tool invocations survive `cd` in recipes
BIN_DIR     := $(abspath bin)
TOOLS_DIR   := $(BIN_DIR)/tools

# Tool versions
CONTROLLER_GEN_VERSION  ?= v0.16.4
KUSTOMIZE_VERSION       ?= v5.4.3
GOLANGCI_LINT_VERSION   ?= v1.61.0
ENVTEST_VERSION         ?= release-0.19

# Tool paths
CONTROLLER_GEN  := $(TOOLS_DIR)/controller-gen
KUSTOMIZE       := $(TOOLS_DIR)/kustomize
GOLANGCI_LINT   := $(TOOLS_DIR)/golangci-lint
ENVTEST         := $(TOOLS_DIR)/setup-envtest

# Go build flags
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     ?= -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(GIT_COMMIT) \
	-X main.buildDate=$(BUILD_DATE)

## Default target — build and lint
.DEFAULT_GOAL := help

# --------------------------------------------------------------------------- #
#  Help                                                                         #
# --------------------------------------------------------------------------- #

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_\/-]+:.*?##/ { printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# --------------------------------------------------------------------------- #
#  Development                                                                  #
# --------------------------------------------------------------------------- #

.PHONY: all
all: generate fmt vet build ## Generate, format, vet, and build

.PHONY: build
build: ## Build the operator binary
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/manager ./main.go

.PHONY: run
run: generate fmt vet ## Run the operator locally against the active kubeconfig cluster
	go run ./main.go --leader-elect=false

.PHONY: fmt
fmt: ## Run go fmt
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

# --------------------------------------------------------------------------- #
#  Code generation                                                              #
# --------------------------------------------------------------------------- #

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate DeepCopy methods (overwrites zz_generated_deepcopy.go)
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate CRD and RBAC manifests into config/
	$(CONTROLLER_GEN) \
	  rbac:roleName=manager-role \
	  crd \
	  webhook \
	  paths="./..." \
	  output:crd:artifacts:config=config/crd/bases \
	  output:rbac:artifacts:config=config/rbac

# --------------------------------------------------------------------------- #
#  Testing                                                                      #
# --------------------------------------------------------------------------- #

.PHONY: test
test: generate fmt vet $(ENVTEST) ## Run unit and controller tests with envtest
	KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_VERSION) --bin-path $(TOOLS_DIR)/envtest -p path)" \
	  go test ./... -coverprofile cover.out -v

.PHONY: test-unit
test-unit: ## Run unit tests only (no envtest required)
	go test ./internal/... -v

.PHONY: cover
cover: test ## Open test coverage report in the browser
	go tool cover -html=cover.out

# --------------------------------------------------------------------------- #
#  Docker                                                                       #
# --------------------------------------------------------------------------- #

.PHONY: docker-build
docker-build: ## Build the operator container image (IMG=pingone-operator:latest)
	docker build --platform $(PLATFORM) \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg GIT_COMMIT=$(GIT_COMMIT) \
	  --build-arg BUILD_DATE=$(BUILD_DATE) \
	  -t $(IMG) .

# docker-desktop target: build and make the image available to the local
# Docker Desktop Kubernetes cluster without needing a registry.
.PHONY: docker-desktop
docker-desktop: docker-build ## Build image and deploy to Docker Desktop Kubernetes
	$(MAKE) install
	$(MAKE) deploy

.PHONY: docker-push
docker-push: ## Push the operator container image to a remote registry
	docker push $(IMG)

.PHONY: docker-buildx
docker-buildx: ## Build and push a multi-arch image via buildx
	docker buildx build \
	  --platform linux/amd64,linux/arm64 \
	  --push \
	  -t $(IMG) .

# --------------------------------------------------------------------------- #
#  Helm / OCI                                                                  #
# --------------------------------------------------------------------------- #

.PHONY: helm-sync-crds
helm-sync-crds: manifests ## Copy generated CRD manifests into the Helm chart crds/ directory
	cp config/crd/bases/*.yaml $(HELM_CHART_DIR)/crds/

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint $(HELM_CHART_DIR)

.PHONY: helm-package
helm-package: ## Package the Helm chart into dist/
	mkdir -p $(HELM_PACKAGE_DIR)
	helm package $(HELM_CHART_DIR) --destination $(HELM_PACKAGE_DIR)

.PHONY: helm-oci-push
helm-oci-push: helm-package ## Push the Helm chart package to an OCI registry (set HELM_OCI_REPO)
	helm push $(HELM_PACKAGE_DIR)/pingone-operator-$(HELM_VERSION).tgz $(HELM_OCI_REPO)

.PHONY: helm-oci-install
helm-oci-install: ## Install/upgrade from an OCI chart reference
	helm upgrade --install pingone-operator $(HELM_OCI_REPO)/pingone-operator \
	  --version $(HELM_VERSION) \
	  --namespace pingone-system \
	  --create-namespace

# --------------------------------------------------------------------------- #
#  Cluster — CRDs and RBAC                                                      #
# --------------------------------------------------------------------------- #

.PHONY: install
install: manifests $(KUSTOMIZE) ## Install CRDs into the active cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests $(KUSTOMIZE) ## Uninstall CRDs from the active cluster
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=true -f -

.PHONY: deploy
deploy: manifests $(KUSTOMIZE) ## Deploy the operator to the active cluster (uses IMG)
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: $(KUSTOMIZE) ## Remove the operator from the active cluster
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=true -f -

.PHONY: sample
sample: ## Apply the sample PingEnvironment CR
	kubectl apply -f config/samples/pingone_v1alpha1_pingenvironment.yaml

.PHONY: sample-delete
sample-delete: ## Delete the sample PingEnvironment CR
	kubectl delete -f config/samples/pingone_v1alpha1_pingenvironment.yaml --ignore-not-found=true

.PHONY: example-getting-started
example-getting-started: ## Apply the getting-started example (edit credentials first)
	kubectl apply -f examples/getting-started/cert-manager.yaml
	kubectl apply -f examples/getting-started/environment.yaml

.PHONY: example-getting-started-delete
example-getting-started-delete: ## Remove the getting-started example
	kubectl delete -f examples/getting-started/environment.yaml --ignore-not-found=true
	kubectl delete -f examples/getting-started/cert-manager.yaml --ignore-not-found=true

# --------------------------------------------------------------------------- #
#  Tools — downloaded into bin/tools/                                           #
# --------------------------------------------------------------------------- #

$(TOOLS_DIR):
	mkdir -p $(TOOLS_DIR)

$(CONTROLLER_GEN): $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) go install \
	  sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

$(KUSTOMIZE): $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) go install \
	  sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

$(GOLANGCI_LINT): $(TOOLS_DIR)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
	  | sh -s -- -b $(abspath $(TOOLS_DIR)) $(GOLANGCI_LINT_VERSION)

$(ENVTEST): $(TOOLS_DIR)
	GOBIN=$(abspath $(TOOLS_DIR)) go install \
	  sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: tools
tools: $(CONTROLLER_GEN) $(KUSTOMIZE) $(GOLANGCI_LINT) $(ENVTEST) ## Download all dev tools

# --------------------------------------------------------------------------- #
#  Clean                                                                        #
# --------------------------------------------------------------------------- #

.PHONY: clean
clean: ## Remove build artefacts (bin/, cover.out)
	rm -rf $(BIN_DIR) cover.out

.PHONY: clean-cache
clean-cache: ## Remove the Helm chart cache (/tmp/helm-cache)
	rm -rf /tmp/helm-cache

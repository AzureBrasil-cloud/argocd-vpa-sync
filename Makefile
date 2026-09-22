BINARY := argocd-vpa-updater
CRD_API_PATH := ./internal/apis/vpagitopsbinding/v1alpha1/...

.PHONY: build build-dist test vet fmt web-build web-dev run run-dist manifests generate crds

build:
	go build -o bin/$(BINARY) ./cmd/argocd-vpa-updater

# Full build with the dashboard embedded (-tags dist; see web/embed_dist.go).
# Requires Node -- this is what the Dockerfile does.
build-dist: web-build
	go build -tags dist -o bin/$(BINARY) ./cmd/argocd-vpa-updater

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

run: build
	./bin/$(BINARY)

run-dist: build-dist
	./bin/$(BINARY)

web-build:
	cd web && npm install && npm run build

web-dev:
	cd web && npm install && npm run dev

# Renders the Kubernetes manifests without applying them, for review.
manifests:
	kubectl kustomize deploy/manifests

# Regenerates zz_generated.deepcopy.go from +kubebuilder:object:... markers
# on the VpaGitOpsBinding CRD types.
generate:
	go tool controller-gen object:headerFile="" paths="$(CRD_API_PATH)"

# Regenerates the CRD YAML from the same markers into deploy/manifests/crds.
crds:
	go tool controller-gen crd:crdVersions=v1,allowDangerousTypes=true paths="$(CRD_API_PATH)" output:crd:artifacts:config=deploy/manifests/crds

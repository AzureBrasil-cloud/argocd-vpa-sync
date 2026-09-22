# argocd-vpa-updater

A GitOps bridge for Kubernetes Vertical Pod Autoscaler (VPA) recommendations,
inspired by [Argo CD Image Updater](https://argocd-image-updater.readthedocs.io/)
but applied to container `resources` instead of image tags.

## Why

VPAs in `updateMode: Auto` rewrite `resources.requests`/`limits` directly on
Pods, which makes Git stop reflecting what's actually running and causes
Argo CD to fight the VPA on every sync. `argocd-vpa-updater` keeps VPAs in
`updateMode: Off` (recommendation-only), shows their recommendations next to
what's currently requested by the live workload, and — once a human selects
and confirms a recommendation — writes the change back to Git so Argo CD
picks it up and syncs it through the normal GitOps flow.

```
VPA recommendation → seleção no dashboard → commit no Git → sync do Argo CD → rollout normal
```

No Pod, Deployment or StatefulSet is ever modified directly by this
controller — the only path to a change is a Git commit.

## Current scope (this phase)

This is the first implementation phase. It delivers:

- The project's core contracts (see [Architecture](#architecture) below).
- A CRD (`VpaGitOpsBinding`) that opts a VPA in and configures its write-back
  target; a controller that watches these bindings and normalizes the
  referenced VPAs' recommendations.
- A **read-only** HTTP API and dashboard: list and detail views comparing
  the VPA recommendation to the value currently requested by the live
  workload (Deployment/StatefulSet/CronJob) it targets, with delta and
  eligibility computed per container. This is deliberately not the value
  declared in Git: a pending or stuck Argo CD sync can leave the two
  briefly disagreeing, and reconciling that gap is this project's own job,
  not a precondition for showing a useful number.
- A minimal Kubernetes Secret–backed `StateStore`.
- A **fake, fully local** `GitWriteBackService` (real Git operations via
  [go-git](https://github.com/go-git/go-git) against a local repository) —
  this exercises the entire commit/branch/idempotency/conflict-detection
  logic, but is not yet wired to the dashboard or to a real GitHub/GitLab/
  Azure DevOps integration.

**Not yet implemented:** the dashboard's selection/apply UI and API
endpoints, real Git provider write-back (GitHub/GitLab/Azure DevOps),
Kustomize-patch support, and the full governance rule engine (HPA-CPU
warning, per-VPA increase/decrease thresholds beyond `minChangePercent`).
Interfaces are already shaped so none of this requires breaking changes.

## Architecture

```
cmd/argocd-vpa-updater/   entry point: wires everything below, runs the
                          controller-runtime manager and the HTTP API side
                          by side in one process

internal/
  domain/                 core types shared by every package (no k8s/git deps)
  vpaapi/                 minimal local VerticalPodAutoscaler API types
                          (autoscaling.k8s.io/v1), hand-written to avoid
                          pulling the full k8s.io/autoscaler module
  apis/vpagitopsbinding/v1alpha1/
                          the VpaGitOpsBinding CRD's Go types (codegen'd
                          deepcopy + CRD YAML via controller-gen, see
                          `make generate`/`make crds`)
  controller/             reconciler: watches VpaGitOpsBinding CRs (and, as a
                          secondary watch, the VPAs they reference), writing
                          normalized state into vparecommendation.Cache
  vparecommendation/      VpaRecommendationReader contract + in-memory cache
  resolver/                GitOpsTargetResolver: a binding's spec -> WriteTarget
  argocdapp/              reads the Argo CD Application CR (repo-url cross-check only)
  workloadresources/      reads the current resources.requests straight off
                          the live Deployment/StatefulSet/CronJob a VPA
                          targets -- this, not Git, is what the dashboard
                          diffs a recommendation against
  patcher/                ManifestPatcher: edits resources.requests/limits in
                          a file's content, preserving everything else
  credentials/            RepositoryCredentialsProvider: reads Argo CD's own
                          repository credential Secrets
  gitexec/                shells out to the real `git` binary (see note below)
  gitrepo/                read-only Git access to a file's declared content,
                          with a short-TTL cache -- used by write-back to
                          read what it's about to patch, not by the dashboard
  gitwriteback/           GitWriteBackService contract + FakeGitWriteBackService
  gittest/                shared test helpers for local Git repos
  statestore/             StateStore contract + Secret-backed implementation
                          with optimistic concurrency and retention/compaction
  eligibility/            delta + minimum-change-threshold + eligibility rules
  idempotency/            deterministic key for "this exact recommendation,
                          applied to this exact target"
  api/                    read-only HTTP API consumed by the dashboard

web/                      React + Vite dashboard (SPA)
deploy/manifests/         Kubernetes manifests (RBAC, Deployment, Service, state Secret)
test/integration/         end-to-end test against a real local Git repository
```

### Why `git` CLI instead of a pure-Go Git library

`internal/gitrepo` (the read path write-back uses to fetch a file before
patching it) shells out to the real `git` binary via `internal/gitexec`
rather than using
[go-git](https://github.com/go-git/go-git). This was a deliberate switch
after production testing: go-git failed against a real Azure DevOps-hosted
repository with an opaque `object not found` error -- the resolved branch
ref pointed at a commit that wasn't actually present in what go-git fetched
-- across shallow, full, and single-branch clone variations, with no
network error or OOM involved. The organization's own git-based tooling
(which shells out to real git) works fine against the same repository. As a
consequence, the runtime container is **not** distroless: it needs `git` and
an SSH client installed (see `Dockerfile`). `internal/gitwriteback`'s fake
implementation still uses go-git internally, since it only ever operates
against local (`file://`) repositories in tests, where the incompatibility
doesn't apply.

### The core contracts

| Contract | Package | Purpose |
|---|---|---|
| `VpaRecommendationReader` | `internal/vparecommendation` | List every opted-in VPA, normalized |
| `GitOpsTargetResolver` | `internal/resolver` | VPA + container -> resolved Git write target |
| `workloadresources.Reader` | `internal/workloadresources` | VPA's live workload + container -> current resources.requests |
| `ManifestPatcher` | `internal/patcher` | Read/patch resource values in a file, preserving structure |
| `RepositoryCredentialsProvider` | `internal/credentials` | Resolve Git credentials without duplicating Argo CD's own |
| `GitWriteBackService` | `internal/gitwriteback` | Branch/commit/PR, idempotently, without force-overwriting conflicts |
| `StateStore` | `internal/statestore` | Persist small operational state, swappable backend |

## Opting a VPA in

A VPA is only processed if a `VpaGitOpsBinding` CR in the **same namespace**
references it by name — the CR's existence is the opt-in signal, there is no
separate "enabled" flag. This mirrors the CRD-based pattern already used by
this organization's own `argocd-image-updater` fork rather than an
annotation-based one. Every piece of Git write-back configuration is
explicit — nothing is inferred from the VPA or Deployment name:

```yaml
apiVersion: argocd-vpa-updater.argoproj.io/v1alpha1
kind: VpaGitOpsBinding
metadata:
  name: checkout-api-vpa-sync
  namespace: payments
spec:
  vpaRef:
    name: checkout-api-vpa
  argoCDApplicationRef:                # optional; only used for a repo-url cross-check warning
    name: payments-api
  repoURL: https://git.example.com/team/app.git
  repoBranch: main
  writeBackPolicy: pull-request        # commit | pull-request (default: commit)
  minChangePercent: 10                 # default eligibility threshold; overridable per container
  containers:
    - name: app
      manifestType: helm-values        # helm-values | kustomize-patch | yaml
      manifestPath: apps/payments/values-prd.yaml
      cpuKeyPath: resources.requests.cpu
      memoryKeyPath: resources.requests.memory
```

`containers` is a list, so one binding can configure write-back for more
than one container of the VPA's target workload. `cpuKeyPath`/`memoryKeyPath`
support indexing into a container list by name, e.g.
`spec.template.spec.containers[app].resources.requests.cpu` for a plain
Deployment manifest; a Helm values file typically uses a plain dotted path
like `resources.requests.cpu`.

`status.conditions[type=Ready]` reports validation problems (missing
required field, unknown enum value, the referenced VPA not found, or
`updateMode != Off`) natively via `kubectl get vpagitopsbindings` /
`kubectl describe`, alongside the same information surfaced in the
dashboard.

## Credentials

`argocd-vpa-updater` never stores its own Git tokens. It reads the
repository credential Secrets Argo CD already manages, in Argo CD's own
namespace, matching the same labels Argo CD itself uses
(`argocd.argoproj.io/secret-type: repository|repo-creds`). See
`internal/credentials/argocd_secret_provider.go`.

## RBAC

See `deploy/manifests/` for the exact manifests. Summary:

| Scope | Resource | Verbs |
|---|---|---|
| cluster | `autoscaling.k8s.io/verticalpodautoscalers` | get, list, watch |
| cluster | `argoproj.io/applications` | get, list, watch |
| cluster | `argocd-vpa-updater.argoproj.io/vpagitopsbindings` | get, list, watch |
| cluster | `argocd-vpa-updater.argoproj.io/vpagitopsbindings/status` | get, update, patch |
| cluster | `apps/deployments`, `apps/statefulsets` | get |
| cluster | `batch/cronjobs` | get |
| `argocd-vpa-updater` namespace | `secrets/argocd-vpa-updater-state` (by name) | get, update |
| `argocd` namespace | `secrets` | get, list, watch |

The Deployment/StatefulSet/CronJob grant is `get` only, never `list`/`watch`:
the dashboard reads one named workload at a time to show the container
resources currently running (`internal/workloadresources`), it never
enumerates them, and its client has caching disabled for these types so no
cluster-wide watch is ever attempted.

The `argocd` namespace grant is broader than ideal — Kubernetes RBAC cannot
filter `list`/`watch` on Secrets by label — see the comment in
`deploy/manifests/04-role-repo-creds.yaml` for the full rationale and
follow-up options. The application code itself only ever reads
labeled repository-credential Secrets and never logs or returns their
content (`domain.GitCredentials` redacts itself in every serialization
path).

## Running locally

```sh
go build ./cmd/argocd-vpa-updater
KUBECONFIG=... ./argocd-vpa-updater     # needs a cluster with the VPA CRDs installed

cd web && npm install && npm run dev    # dashboard, proxies /api to :8080
```

## Testing

```sh
go build ./... && go vet ./... && go test ./...
cd web && npm run build
```

Unit tests cover delta/eligibility calculation, idempotency key derivation,
binding validation, target resolution (including multi-container bindings),
YAML/Helm-values patching (including comment/structure preservation), state
retention and optimistic concurrency, and the reconciler (including its
field-indexed secondary watch on VerticalPodAutoscaler, which must not leak
across namespaces). `test/integration` exercises the
fake write-back service end to end against a real local Git repository:
applying a recommendation, replaying it idempotently (no duplicate commit),
and a genuine concurrent-edit race that must be rejected as a conflict
without partially writing anything.

## Deploying

```sh
kubectl apply -k deploy/manifests
```

This creates the `VpaGitOpsBinding` CRD, the `argocd-vpa-updater` namespace,
ServiceAccount, RBAC, a bootstrapped (empty) state Secret, and the
Deployment/Service. It assumes Argo CD itself lives in the `argocd`
namespace (override via the `ARGOCD_NAMESPACE` env var on the Deployment if
not).

`deploy/manifests/overlays/argocd-namespace-test/` is a temporary variant
that installs everything into the `argocd` namespace instead (used for
early smoke-testing where reusing that namespace was convenient); prefer
the base manifests above once the project is onboarded as a proper Argo CD
Application.

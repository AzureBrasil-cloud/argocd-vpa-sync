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

## Current scope

- A CRD (`VpaGitOpsBinding`) that opts a VPA in and configures its write-back
  target; a controller that watches these bindings and normalizes the
  referenced VPAs' recommendations.
- A dashboard and HTTP API, behind an [admin login](#authentication): list
  and detail views comparing the VPA recommendation to the value currently
  requested by the live workload (Deployment/StatefulSet/CronJob) it
  targets, with delta and eligibility computed per container. This is
  deliberately not the value declared in Git: a pending or stuck Argo CD
  sync can leave the two briefly disagreeing, and reconciling that gap is
  this project's own job, not a precondition for showing a useful number.
- Selecting recommendations (one or in bulk), with request headroom and
  limit settings, queues them in a Kubernetes Secret-backed `StateStore`; a
  write-back worker then commits and pushes them to Git with the real `git`
  CLI, using Argo CD's own repository credentials.

**Not yet implemented:** opening the pull request itself for
`writeBackPolicy: pull-request` (the topic branch is pushed, the PR is not
created), Kustomize-patch support, and the full governance rule engine
(HPA-CPU warning, per-VPA increase/decrease thresholds beyond
`minChangePercent`). Interfaces are already shaped so none of this requires
breaking changes.

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
  auth/                   admin password hashes (bcrypt/argon2id), signed
                          session tokens, login throttling
  api/                    HTTP API consumed by the dashboard

web/                      React + Vite dashboard (SPA)
deploy/helm/argocd-vpa-sync/
                          Helm chart (CRD, RBAC, Deployment, Service, auth Secret)
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
      # never inferred -- omit to leave that limit untouched
      cpuLimitKeyPath: resources.limits.cpu
      memoryLimitKeyPath: resources.limits.memory
```

`containers` is a list, so one binding can configure write-back for more
than one container of the VPA's target workload. `cpuKeyPath`/`memoryKeyPath`
support indexing into a container list by name, e.g.
`spec.template.spec.containers[app].resources.requests.cpu` for a plain
Deployment manifest; a Helm values file typically uses a plain dotted path
like `resources.requests.cpu`.

### Limits

When a container declares `cpuLimitKeyPath` / `memoryLimitKeyPath`,
write-back also rewrites that limit, so a new request can never exceed the
limit already in Git (which Kubernetes rejects with `requests: Invalid
value: ... must be less than or equal to memory limit`). The dashboard lets
you choose, per resource, how the limit is set:

- **Headroom %** (default 20%): the new request becomes `(100 - headroom)%`
  of the limit, i.e. `limit = request / (1 - headroom/100)`, rounded up.
  A 305Mi request at 20% gets `305Mi / 0.8 = 381.25Mi -> 382Mi`. Must be
  `>= 0` and `< 100`.
- **Absolute value**: the limit is written exactly as given, in any
  Kubernetes quantity valid for the resource (memory: `512Mi`, `1Gi`,
  `1.5G`...; CPU: `500m`, `0.5`, `2`). It must not be below the new request.

Notes:

- The limit is always recalculated, so it can go **down** as well as up;
  the summary flags every limit that will be lowered.
- Limit key paths are **never inferred** from the request key paths (a Helm
  values layout can put the limit anywhere). Without them, only the request
  is written and the dashboard shows a warning.
- A limit the file doesn't declare is left absent, never added.

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

For SSH-authenticated repositories, host key verification is always strict
(`StrictHostKeyChecking=yes`) — there is no insecure fallback. The known_hosts
data comes from either the matched Secret's `sshKnownHosts` field, or, since
that field is rarely populated on Argo CD's own repo Secrets, from Argo CD's
central `argocd-ssh-known-hosts-cm` ConfigMap (mounted as `optional: true`, so
its absence never blocks the pod from starting), pointed at via
`SSH_KNOWN_HOSTS` (see the chart's `ssh.*` values and
`internal/gitexec/runner.go`). That ConfigMap can only be mounted when the
chart is installed in Argo CD's namespace; elsewhere, set
`ssh.extraKnownHosts`. This mirrors the same ConfigMap/mount
convention as the upstream `argocd-image-updater` Helm chart, which is why
that controller has never needed extra known_hosts configuration here either.
A host missing from both — e.g. a
`git ls-remote`/writeback failing with `Host key verification failed` for a
host Argo CD itself syncs fine — means that ConfigMap needs the host's key
added on the Argo CD side (`argocd cert add-ssh --batch` or Argo CD's
Settings → Certificates UI).

## RBAC

See `deploy/helm/argocd-vpa-sync/templates/rbac-*.yaml` for the exact
rules. Summary:

| Scope | Resource | Verbs |
|---|---|---|
| cluster | `autoscaling.k8s.io/verticalpodautoscalers` | get, list, watch |
| cluster | `argoproj.io/applications` | get, list, watch |
| cluster | `argocd-vpa-updater.argoproj.io/vpagitopsbindings` | get, list, watch |
| cluster | `argocd-vpa-updater.argoproj.io/vpagitopsbindings/status` | get, update, patch |
| cluster | `apps/deployments`, `apps/statefulsets` | get |
| cluster | `batch/cronjobs` | get |
| release namespace | `secrets/<release>-state` (by name) | get, update |
| release namespace | `secrets` | create |
| Argo CD namespace | `secrets` | get, list, watch |

The Deployment/StatefulSet/CronJob grant is `get` only, never `list`/`watch`:
the dashboard reads one named workload at a time to show the container
resources currently running (`internal/workloadresources`), it never
enumerates them, and its client has caching disabled for these types so no
cluster-wide watch is ever attempted.

The controller creates its state Secret on first use, so its content is
never owned (or reset) by Helm or Argo CD. RBAC cannot scope `create` by
name, hence the namespace-wide `create`; it grants no read access.

The Argo CD namespace grant is broader than ideal — Kubernetes RBAC cannot
filter `list`/`watch` on Secrets by label — see the comment in
`templates/rbac-repo-creds.yaml` for the full rationale and follow-up
options. The application code itself only ever reads
labeled repository-credential Secrets and never logs or returns their
content (`domain.GitCredentials` redacts itself in every serialization
path).

## Running locally

```sh
go build ./cmd/argocd-vpa-updater
# needs a cluster with the VPA CRDs installed; see Authentication below to
# run with a login instead
KUBECONFIG=... AUTH_ENABLED=false ./argocd-vpa-updater

cd web && npm install && npm run dev    # dashboard, proxies /api to :8080
```

## Testing

```sh
go build ./... && go vet ./... && go test ./...
cd web && npm run build
helm lint deploy/helm/argocd-vpa-sync
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

The Helm chart is published as an OCI artifact on Docker Hub, and the image
as `azurebrasil/argocd-vpa-updater` (linux/amd64 and linux/arm64):

```sh
helm install argocd-vpa-sync oci://registry-1.docker.io/azurebrasil/argocd-vpa-sync \
  --version 0.1.0 --namespace argocd
```

Installing into Argo CD's own namespace (`argocd` by default) is the
simplest option: it lets the pod mount Argo CD's `argocd-ssh-known-hosts-cm`.
Anywhere else, set `argocd.namespace` to where Argo CD runs and
`ssh.extraKnownHosts` to the SSH host keys to trust. See
[`values.yaml`](deploy/helm/argocd-vpa-sync/values.yaml) for every option.

Helm installs the `VpaGitOpsBinding` CRD from the chart's `crds/` directory
on first install but, by design, never upgrades or deletes it. After an
upgrade that changes the CRD, apply it yourself:

```sh
helm pull oci://registry-1.docker.io/azurebrasil/argocd-vpa-sync --version <version> --untar
kubectl apply --server-side -f argocd-vpa-sync/crds/
```

### Authentication

Every `/api/` route requires a session from a single local admin account,
modelled on Argo CD's `admin` user: the password is only ever stored as a
**bcrypt** or **argon2id** hash, and a login is exchanged for an HMAC-signed
session token (an `HttpOnly`, `SameSite=Strict` cookie; API clients can send
the `token` returned by `POST /api/v1/session` as `Authorization: Bearer`).
Sessions last `auth.sessionTTL` (24h), and changing the password hash
invalidates every existing session. Failed logins are throttled per client
IP.

There are three ways to set the password:

1. **Generated (default).** On install the chart generates a random
   password, stores its bcrypt hash in `<release>-secret`, and the plain
   text in `<release>-initial-admin-secret`, like Argo CD's
   `argocd-initial-admin-secret`:

   ```sh
   kubectl -n argocd get secret argocd-vpa-sync-initial-admin-secret \
     -o jsonpath='{.data.password}' | base64 -d; echo
   ```

2. **Your own hash**, via `auth.admin.passwordHash`. The initial-admin
   Secret is removed once this is set.

   ```sh
   # bcrypt
   htpasswd -nbBC 10 "" 'my-password' | tr -d ':\n'
   argocd account bcrypt --password 'my-password'
   # argon2id
   echo -n 'my-password' | argon2 "$(openssl rand -hex 16)" -id -e
   ```

3. **An existing Secret**, via `auth.existingSecret`, with the keys
   `admin.password` (the hash) and `server.secretkey` (the session signing
   key, at least 32 characters). The pod reads it at startup, so run
   `kubectl rollout restart` after rotating it.

**Deploying the chart with Argo CD** (or any `helm template` flow): Helm's
`lookup` returns nothing there, so options 1 and 2 would regenerate a
password or signing key on every render. Use `auth.existingSecret`, or set
both `auth.admin.passwordHash` and `auth.sessionSigningKey`.

The session cookie is `Secure` by default, so browsers only send it over
HTTPS (and `http://localhost`, so `kubectl port-forward` works). If the
dashboard is served over plain HTTP on another host, set
`auth.cookieSecure=false`.

`auth.enabled=false` disables authentication. The dashboard and the
endpoints that queue Git commits are then open to anyone who can reach the
Service, so only use it for local development.

Running the binary directly, the same settings are environment variables:
`AUTH_ENABLED`, `ADMIN_USERNAME` (`admin`), `ADMIN_PASSWORD_HASH`,
`SESSION_SIGNING_KEY`, `SESSION_TTL` (`24h`) and `COOKIE_SECURE` (`true`).

### Releasing

- **Image:** push a `vX.Y.Z` tag. `docker-publish.yml` publishes `X.Y.Z`,
  `X.Y` and `latest`, and every push to `master` publishes `edge`. Keep the
  chart's `appVersion` set to the image version it ships.
- **Chart:** bump `version` in `deploy/helm/argocd-vpa-sync/Chart.yaml`.
  `chart-publish.yml` pushes it on merge to `master` and never overwrites a
  version that is already published.

Both workflows need the `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`
repository secrets.

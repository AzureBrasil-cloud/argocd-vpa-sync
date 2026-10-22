# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Platform, SRE and DevOps teams that run Argo CD GitOps. They open the dashboard periodically to review Vertical Pod Autoscaler recommendations in batches, pick the CPU and memory changes worth making, and send them to Git. The goal is fewer OOMKills and less wasted capacity, without manual copy-paste of VPA numbers into values files.

## Product Purpose

argocd-vpa-sync lets teams right-size Kubernetes workloads with VPA recommendations without giving up GitOps. VPA `Auto` mode fights Argo CD by changing Pods outside Git; VPA `Off` mode only records recommendations. This product closes the gap: recommendation → review in the dashboard → Git commit → Argo CD sync → normal rollout.

Success: a reviewer can tell at a glance which containers need a change, why others don't, and confidently apply a batch that lands as clean commits.

## Positioning

Nothing in the cluster is changed directly. Every change is a Git commit that patches the workload's values or manifest file in place (keeping comments and formatting), pushed with Argo CD's existing repo credentials. Recommendations are compared with the live workload, not with Git.

## Operating Context

- A team opts a VPA in with a `VpaGitOpsBinding` CRD (VPA must use `updateMode: "Off"`). The binding names the repo, branch, manifest type (`helm-values` or `yaml`), file path and the key paths for requests and, optionally, limits.
- The reviewer selects containers and resources; selections are queued, and a background worker (default poll 20s) commits and pushes.
- Installed with Helm into the `argocd` namespace; usually reached through port-forward on 8080 or an optional Ingress.
- Workloads covered: Deployment, StatefulSet, CronJob, including ones that also use an HPA.

## Capabilities and Constraints

- Eligibility (all must hold): a recommendation exists and the workload has a current request; the recommendation is within the VPA's `minAllowed`/`maxAllowed`; the change is at least `minChangePercent` (default 10%, per binding or per container); the current request is outside the VPA's `lowerBound`..`upperBound`. Non-eligible rows explain why.
- Limits: set by headroom % (default 20%) or an absolute value. A new request never ends up above its limit. Limit key paths are never guessed: without them only the request is written and the UI warns. Limits that go down are flagged.
- Write-back statuses: `new`, `selected`, `applying`, `applied`, `skipped`, `failed`, with commit SHA and time once applied.
- Auth is off by default. Setting an admin password hash turns on a single admin login (24h session cookie; Bearer token for API clients).
- The frontend is a React + Vite SPA embedded in the Go binary via `go:embed`. UI copy is English only today.
- Terminology to keep: recommendation, eligible / not eligible, write-back, binding, live workload, request, limit, headroom, lower/upper bound, mode.
- The user set no further constraints for the frontend refactor: language, serving model, accessibility level and branding are open to change.

## Brand Commitments

- Official name: **argocd-vpa-sync** (repo, Helm chart, Service). The image, binary, CRD group and current UI title still say `argocd-vpa-updater`; the UI should move to the official name. Owner: AzureBrasil-cloud. License: MIT.
- No logo exists; the header uses a text glyph. No binding visual identity.

## Evidence on Hand

- Current dashboard screenshots: `docs/images/dashboard-list.png`, `dashboard-cards.png`, `selection-summary.png`, `recommendation-detail.png`, `login.png`.
- README.md describes the flow and eligibility rules.
- No customers, testimonials, benchmarks or adoption numbers exist; do not invent them.

## Product Principles

1. Git is the only path to the cluster. The UI never implies it changes workloads directly.
2. Explain every decision: why a row is or isn't eligible, and what exactly will be written.
3. Built for batch review: scanning many workloads and acting on a selection beats deep single-item flows.
4. Safe by default: never guess, warn on missing limit paths and on lowered limits.

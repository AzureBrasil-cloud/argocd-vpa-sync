# syntax=docker/dockerfile:1

# --- frontend build stage -------------------------------------------------
FROM node:22-alpine AS webbuilder
WORKDIR /web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ .
RUN npm run build

# --- build stage -----------------------------------------------------------
FROM golang:1.26-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git ca-certificates

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed_dist.go web/embed_stub.go web/
COPY --from=webbuilder /web/dist web/dist

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# -tags dist embeds the dashboard build (web/dist, copied above) into the
# binary via web/embed_dist.go; see that file's doc comment for why plain
# `go build` (no tags) intentionally does not require it.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -tags dist \
    -trimpath \
    -ldflags "-s -w \
      -X github.com/azurebrasil/argocd-vpa-updater/internal/version.Version=${VERSION} \
      -X github.com/azurebrasil/argocd-vpa-updater/internal/version.GitCommit=${GIT_COMMIT} \
      -X github.com/azurebrasil/argocd-vpa-updater/internal/version.BuildDate=${BUILD_DATE}" \
    -o /out/argocd-vpa-updater \
    ./cmd/argocd-vpa-updater

# --- runtime stage --------------------------------------------------------
# NOT distroless: internal/gitrepo shells out to the real `git` binary
# (internal/gitexec) instead of a pure-Go Git implementation, because go-git
# was found in production to fail against at least one real Azure DevOps
# repository ("object not found" even on a full, valid clone) while real git
# works fine against the same repo. That requires git + an ssh client to be
# present at runtime, which rules out a shell-less distroless base.
FROM alpine:3.20 AS runtime

RUN apk add --no-cache git openssh-client ca-certificates \
    && adduser -D -u 1000 appuser

COPY --from=builder /out/argocd-vpa-updater /argocd-vpa-updater

USER appuser
EXPOSE 8080
ENTRYPOINT ["/argocd-vpa-updater"]

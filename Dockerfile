# =============================================================================
# Dockerfile — multi-stage build for the art-gallery Go binary
# =============================================================================
#
# Identical pattern to sbx-apps/online-shop. Uses direct docker.io/gcr.io
# paths (NOT the AR remote dockerhub-remote/quay-remote) because GitLab CI's
# docker:dind pulls FROM images BEFORE before_script runs — at which point
# AR auth isn't set up yet. The runtime is distroless static + non-root.

# ---- Stage 1: builder ----
FROM golang:1.23-alpine AS builder
WORKDIR /src
# Copy module files first for layer caching — re-copies on every change in
# real-world apps with deps; for our std-lib-only app it's essentially free.
COPY go.mod ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 + static binary so the distroless static runtime can run it.
# -trimpath strips local paths from the binary (smaller, more reproducible).
# -ldflags="-s -w" strips the Go symbol table + DWARF (smaller binary, no
# debug info — fine for prod; if you want stack traces use a richer base).
RUN CGO_ENABLED=0 go build \
    -trimpath -ldflags="-s -w" \
    -o /out/art-gallery ./...

# ---- Stage 2: runtime ----
# distroless static = just glibc + ca-certs + tzdata, no shell.
# :nonroot tag bakes in USER 65532:65532 (uid/gid). Workload Identity-friendly.
FROM gcr.io/distroless/static-debian12:nonroot

# Pin numeric uid/gid in the manifest's securityContext too — the :nonroot
# tag's `USER nonroot` is non-numeric so admission rejects pods that set
# runAsNonRoot:true without explicit numeric uid. (sbx-02 memory entry
# distroless_needs_numeric_uid.md.)

WORKDIR /
COPY --from=builder /out/art-gallery /art-gallery

EXPOSE 8080
ENTRYPOINT ["/art-gallery"]

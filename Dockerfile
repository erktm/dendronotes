# dendronotes: reproducible multi-stage build
#
# Deliberately without a "# syntax=…" line: Docker's built-in BuildKit
# frontend is sufficient, so the build never pulls a frontend image from
# the internet.
#
# Offline/air-gap: if .offline/npm-cache exists (created by `make vendor`),
# npm installs exclusively from it. Go dependencies come from vendor/.
# Base images are pinned by digest; in an air gap they must come from a
# local registry (override via build args).

ARG NODE_IMAGE=node:26.13.0-alpine@sha256:c6ad9e9c1d80f6eb607c26b4b0a132ff7a2344fad1c78ea3baa2a7d4c8de6d94
ARG GO_IMAGE=golang:1.27.3-alpine@sha256:fa4881266890ef051f0ce63afdfaccb69bbe8186084387f4fafd60d7ecbabc71
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

# --- Frontend (platform-independent, runs on the build platform) -------------
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web
WORKDIR /src/web
ENV NPM_CONFIG_UPDATE_NOTIFIER=false
COPY web/package.json web/package-lock.json web/.npmrc ./
COPY .offline/ /offline/
RUN if [ -d /offline/npm-cache ]; then \
      echo "npm: installing from offline cache"; \
      npm ci --offline --cache /offline/npm-cache; \
    else \
      npm ci; \
    fi
COPY web/ ./
RUN npm run build

# --- Go binary (cross-compiled, no CGO) --------------------------------------
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=vendor
WORKDIR /src
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/dendronotes ./cmd/dendronotes \
 && mkdir -p /out/somas /out/data

# --- Runtime: distroless, no shell, UID 65532 --------------------------------
FROM ${RUNTIME_IMAGE}
ARG VERSION=dev
LABEL org.opencontainers.image.title="dendronotes" \
      org.opencontainers.image.description="Self-hosted Markdown notes" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/erktm/dendronotes" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build /out/dendronotes /usr/local/bin/dendronotes
# /somas holds the notes, one folder per soma; /data the accounts (state.db,
# back it up) and the rebuildable search indexes.
COPY --from=build --chown=65532:65532 /out/somas /somas
COPY --from=build --chown=65532:65532 /out/data /data
ENV DENDRONOTES_SOMAS_DIR=/somas \
    DENDRONOTES_DATA_DIR=/data \
    DENDRONOTES_HTTP_PORT=8080
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD ["/usr/local/bin/dendronotes", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/dendronotes"]
CMD ["serve"]

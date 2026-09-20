# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22-alpine
ARG GO_VERSION=1.25-alpine
ARG ALPINE_VERSION=3.22

FROM node:${NODE_VERSION} AS web-build
WORKDIR /src/web

COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY web/ ./
RUN npm run build

FROM golang:${GO_VERSION} AS go-build
WORKDIR /src

COPY go.* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . ./
COPY --from=web-build /src/web/dist ./internal/webui/dist

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG REVISION=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${REVISION}" \
      -o /out/thorsync ./cmd/thorsync

FROM alpine:${ALPINE_VERSION} AS runtime

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown

LABEL org.opencontainers.image.title="ThorSync" \
      org.opencontainers.image.description="Visual save library and synchronization broker for GBA and NDS saves" \
      org.opencontainers.image.source="https://github.com/apgul/thorsync" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 65532 thorsync \
    && adduser -S -D -H -u 65532 -G thorsync thorsync \
    && mkdir -p \
        /var/lib/thorsync/data \
        /var/lib/thorsync/archive \
        /var/lib/thorsync/sync/thor \
        /var/lib/thorsync/sync/windows \
    && chown -R thorsync:thorsync /var/lib/thorsync

WORKDIR /app
COPY --from=go-build --chown=65532:65532 /out/thorsync /usr/local/bin/thorsync

USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8080/health/live || exit 1

ENTRYPOINT ["/usr/local/bin/thorsync"]

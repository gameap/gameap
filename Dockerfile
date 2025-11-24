# ui builder image
FROM node:24-alpine AS uibuilder

WORKDIR /app

COPY web/frontend/package*.json ./web/frontend/
RUN --mount=type=cache,target=/root/.npm \
    cd /app/web/frontend && npm ci

COPY web ./web
RUN cd /app/web/frontend && npm run build --if-present


# builder image
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=uibuilder /app/web/static/dist /app/web/static/dist

ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X 'github.com/gameap/gameap/internal/application/defaults.Version=${VERSION}' -X 'github.com/gameap/gameap/internal/application/defaults.BuildDate=${BUILD_DATE}'" \
    -o gameap \
    ./cmd/gameap


# production image
FROM alpine:3.21

LABEL org.opencontainers.image.title="GameAP" \
      org.opencontainers.image.description="Game server control panel API" \
      org.opencontainers.image.vendor="GameAP" \
      org.opencontainers.image.url="https://gameap.com" \
      org.opencontainers.image.source="https://github.com/gameap/gameap" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -g 1000 gameap && \
    adduser -D -u 1000 -G gameap gameap

COPY --from=builder --chown=gameap:gameap /app/gameap /usr/bin/gameap

USER gameap

WORKDIR /var/lib/gameap

ENV DATABASE_DRIVER=sqlite \
    DATABASE_URL=file:/db.sqlite \
    HTTP_HOST=0.0.0.0 \
    HTTP_PORT=8025

EXPOSE 8025

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8025/api/health || exit 1

ENTRYPOINT ["/usr/bin/gameap"]
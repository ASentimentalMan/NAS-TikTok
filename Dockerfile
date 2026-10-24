# Build the Vue frontend with its locked Node.js dependencies.
FROM node:22.23.3-alpine3.24 AS client-builder

ARG HTTP_PROXY=
ARG HTTPS_PROXY=
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV NO_PROXY="localhost,127.0.0.1,::1,172.17.0.1"

WORKDIR /app/client
ARG VITE_BASE_URL=/
ENV VITE_BASE_URL=${VITE_BASE_URL}
COPY client/package*.json ./
RUN npm ci --no-audit --no-fund
COPY client/ ./
RUN npm run build

# Build a static Go binary for the production server.
FROM golang:1.24-alpine AS server-builder
WORKDIR /src/server
COPY server/ ./
RUN mkdir -p /out && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nas-tiktok ./cmd/nas-tiktok && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nas-tiktok-init ./cmd/nas-tiktok-init

# Run the Go API and serve the Vue dist from the same process.
FROM alpine:3.21 AS runtime
RUN apk add --no-cache curl && \
    addgroup -S app && adduser -S -G app app
WORKDIR /workspace

ENV APP_NAME=NAS-TikTok \
    LISTEN_PORT=3000 \
    SSL=false \
    ALLOW_IPv6=false \
    MEDIA_ROOT=/workspace/static \
    DB_PATH=/workspace/db/nas-tiktok.db \
    STATIC_ROOT=/workspace/client/dist

RUN mkdir -p /workspace/db && chown app:app /workspace/db && chmod 0755 /workspace/db

EXPOSE 3000
COPY --from=client-builder /app/client/dist /workspace/client/dist
COPY --from=server-builder /out/nas-tiktok /usr/local/bin/nas-tiktok
COPY --from=server-builder /out/nas-tiktok-init /usr/local/bin/nas-tiktok-init

# Allow the initializer to activate CHOWN only when ownership requires repair.
RUN apk add --no-cache --virtual .setcap-deps libcap-utils && \
    setcap cap_chown=p /usr/local/bin/nas-tiktok-init && \
    apk del .setcap-deps

HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
    CMD prefix="${APP_URL_PREFIX:-}"; prefix="${prefix#/}"; prefix="${prefix%/}"; path=/healthz; if [ -n "$prefix" ]; then path="/$prefix$path"; fi; ssl="$(printf '%s' "${SSL:-false}" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')"; scheme=http; if [ "$ssl" = true ]; then scheme=https; fi; curl --fail --silent --show-error --insecure --max-time 3 "$scheme://127.0.0.1:${LISTEN_PORT:-3000}$path" >/dev/null

USER app:app
ENTRYPOINT ["/usr/local/bin/nas-tiktok-init"]

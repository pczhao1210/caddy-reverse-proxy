FROM --platform=$BUILDPLATFORM golang:1.26.8-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/cmd ./cmd
COPY backend/internal ./internal
RUN go test ./...
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/platform ./cmd/server

FROM --platform=$BUILDPLATFORM caddy:2.11.4-builder-alpine@sha256:2b9f32cbba6045e79212bb253f595588d20be8f7d1edf61582c519e124e0d2d8 AS caddy-builder
RUN go install github.com/caddyserver/xcaddy/cmd/xcaddy@v0.4.7
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH XCADDY_SETCAP=0 xcaddy build v2.11.4 --output /usr/bin/caddy \
        --with github.com/caddy-dns/azure@v0.6.0 \
        --replace github.com/caddyserver/certmagic=github.com/caddyserver/certmagic@v0.25.4 \
        --replace github.com/go-chi/chi/v5=github.com/go-chi/chi/v5@v5.3.0 \
        --replace github.com/google/cel-go=github.com/google/cel-go@v0.28.1 \
        --replace github.com/klauspost/compress=github.com/klauspost/compress@v1.18.7 \
        --replace go.opentelemetry.io/otel=go.opentelemetry.io/otel@v1.44.0 \
        --replace google.golang.org/grpc=google.golang.org/grpc@v1.83.2 \
        --replace golang.org/x/crypto=golang.org/x/crypto@v0.57.0 \
        --replace golang.org/x/net=golang.org/x/net@v0.59.0 \
        --replace golang.org/x/text=golang.org/x/text@v0.42.0

FROM alpine:3.22.6@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8
RUN apk add --no-cache ca-certificates tzdata
COPY --from=caddy-builder /usr/bin/caddy /usr/bin/caddy
COPY --from=builder /out/platform /usr/local/bin/platform
RUN caddy version | grep -q '^v2[.]11[.]4 ' \
    && caddy list-modules | grep -qx 'dns.providers.azure' \
    && mkdir -p /data/platform /data/caddy
EXPOSE 80 443 8080
ENV GATEWAY_ROUTES_FILE=/data/platform/routes.json \
    GATEWAY_CERTIFICATE_FILE=/data/platform/certificate.json \
    XDG_DATA_HOME=/data/caddy
ENTRYPOINT ["/usr/local/bin/platform"]

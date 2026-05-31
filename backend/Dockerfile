FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/cmd ./cmd
COPY backend/internal ./internal
RUN go test ./... && CGO_ENABLED=0 GOOS=linux go build -o /out/platform ./cmd/server

FROM caddy:2.10-alpine AS caddy

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=caddy /usr/bin/caddy /usr/bin/caddy
COPY --from=builder /out/platform /usr/local/bin/platform
RUN mkdir -p /data/platform /data/caddy
EXPOSE 80 443 8080
ENV GATEWAY_CONFIG_FILE=/config/platform.json \
    GATEWAY_ROUTES_FILE=/data/platform/routes.json \
    XDG_DATA_HOME=/data/caddy
ENTRYPOINT ["/usr/local/bin/platform"]

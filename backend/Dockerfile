FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS go-base

FROM go-base AS builder
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/cmd ./cmd
COPY backend/internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test ./... && go build -o /out/security-evidence ./cmd/security-evidence
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH; \
    go build -o /out/platform ./cmd/server \
    && go list -deps -json ./cmd/server > /out/platform-packages.json

FROM go-base AS caddy-deps
RUN apk add --no-cache patch jq
COPY backend/patches/step-crypto-jose-v4.patch backend/patches/step-certificates-jose-v4.patch /opt/caddy-patches/
RUN --mount=type=cache,target=/go/pkg/mod \
    set -eu; \
    go mod download -json go.step.sm/crypto@v0.91.1 github.com/smallstep/certificates@v0.30.2 > /tmp/step-modules.json; \
    jq -se 'length == 2 and .[0].Sum == "h1:ZMMdj/A2rQ+Zr8hyCo4BHJd++c5T8VQIXJ8Sljg57p0=" and .[1].Sum == "h1:1G3xBi8sJ740iA1mMPW2Svv7EIZKJ4Zf/iQtA5QlN0Y="' /tmp/step-modules.json; \
    mkdir -p /opt/caddy-deps; \
    cp /tmp/step-modules.json /opt/caddy-deps/modules.json; \
    cp -R "$(go env GOMODCACHE)/go.step.sm/crypto@v0.91.1" /opt/caddy-deps/crypto; \
    cp -R "$(go env GOMODCACHE)/github.com/smallstep/certificates@v0.30.2" /opt/caddy-deps/certificates; \
    chmod -R u+w /opt/caddy-deps; \
    patch --batch --fuzz=0 --forward -d /opt/caddy-deps/crypto -p1 < /opt/caddy-patches/step-crypto-jose-v4.patch; \
    patch --batch --fuzz=0 --forward -d /opt/caddy-deps/certificates -p1 < /opt/caddy-patches/step-certificates-jose-v4.patch
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd /opt/caddy-deps/crypto && go test -mod=mod ./jose/... ./kms/azurekms/... \
    && cd /opt/caddy-deps/certificates && go test -mod=mod ./acme ./acme/api ./cas/stepcas ./authority ./authority/provisioner

FROM caddy-deps AS caddy-builder
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/caddyserver/xcaddy/cmd/xcaddy@v0.4.7
COPY --from=builder /out/security-evidence /usr/local/bin/security-evidence
COPY --from=builder /out/platform /out/platform
COPY --from=builder /out/platform-packages.json /out/platform-packages.json
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /opt/caddy-build \
    && export CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH XCADDY_SKIP_CLEANUP=1 XCADDY_SETCAP=0 TMPDIR=/opt/caddy-build \
    && xcaddy build v2.11.7 --output /usr/bin/caddy \
        --with github.com/caddy-dns/azure@v0.6.0 \
        --replace cel.dev/cel-go=cel.dev/cel-go@v0.33.1 \
        --replace cloud.google.com/go/auth=cloud.google.com/go/auth@v0.24.2 \
        --replace cloud.google.com/go/auth/oauth2adapt=cloud.google.com/go/auth/oauth2adapt@v0.3.1 \
        --replace github.com/DeRuina/timberjack=github.com/DeRuina/timberjack@v1.4.9 \
        --replace github.com/KimMachineGun/automemlimit=github.com/KimMachineGun/automemlimit@v1.0.1 \
        --replace github.com/bits-and-blooms/bitset=github.com/bits-and-blooms/bitset@v1.28.0 \
        --replace github.com/caddyserver/certmagic=github.com/caddyserver/certmagic@v0.25.6 \
        --replace github.com/dlclark/regexp2/v2=github.com/dlclark/regexp2/v2@v2.12.0 \
        --replace github.com/dunglas/httpsfv=github.com/dunglas/httpsfv@v1.1.2 \
        --replace github.com/fxamacker/cbor/v2=github.com/fxamacker/cbor/v2@v2.10.0 \
        --replace github.com/go-chi/chi/v5=github.com/go-chi/chi/v5@v5.5.0 \
        --replace github.com/go-jose/go-jose/v4=github.com/go-jose/go-jose/v4@v4.1.6 \
        --replace github.com/google/certificate-transparency-go=github.com/google/certificate-transparency-go@v1.4.0 \
        --replace github.com/googleapis/enterprise-certificate-proxy=github.com/googleapis/enterprise-certificate-proxy@v0.3.24 \
        --replace github.com/googleapis/gax-go/v2=github.com/googleapis/gax-go/v2@v2.27.0 \
        --replace github.com/grpc-ecosystem/grpc-gateway/v2=github.com/grpc-ecosystem/grpc-gateway/v2@v2.33.0 \
        --replace github.com/hashicorp/go-uuid=github.com/hashicorp/go-uuid@v1.0.4 \
        --replace github.com/huandu/xstrings=github.com/huandu/xstrings@v1.6.3 \
        --replace github.com/klauspost/compress=github.com/klauspost/compress@v1.20.3 \
        --replace github.com/klauspost/cpuid/v2=github.com/klauspost/cpuid/v2@v2.4.1 \
        --replace github.com/mattn/go-colorable=github.com/mattn/go-colorable@v0.1.16 \
        --replace github.com/pandatix/go-abnf=github.com/pandatix/go-abnf@v0.5.3 \
        --replace github.com/pires/go-proxyproto=github.com/pires/go-proxyproto@v0.16.0 \
        --replace github.com/prometheus/client_golang=github.com/prometheus/client_golang@v1.25.0 \
        --replace github.com/prometheus/common=github.com/prometheus/common@v0.73.0 \
        --replace github.com/prometheus/procfs=github.com/prometheus/procfs@v0.23.0 \
        --replace github.com/quic-go/quic-go=github.com/quic-go/quic-go@v0.64.1 \
        --replace github.com/shopspring/decimal=github.com/shopspring/decimal@v1.6.0 \
        --replace github.com/slackhq/nebula=github.com/slackhq/nebula@v1.12.0 \
        --replace github.com/smallstep/certificates=/opt/caddy-deps/certificates \
        --replace github.com/smallstep/scep=github.com/smallstep/scep@v0.0.0-20261014185058-63893dadfda8 \
        --replace go.opentelemetry.io/contrib/exporters/autoexport=go.opentelemetry.io/contrib/exporters/autoexport@v0.72.0 \
        --replace go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp=go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.72.0 \
        --replace go.opentelemetry.io/contrib/propagators/autoprop=go.opentelemetry.io/contrib/propagators/autoprop@v0.72.0 \
        --replace go.opentelemetry.io/otel=go.opentelemetry.io/otel@v1.47.0 \
        --replace go.step.sm/crypto=/opt/caddy-deps/crypto \
        --replace google.golang.org/api=google.golang.org/api@v0.310.0 \
        --replace google.golang.org/genproto/googleapis/rpc=google.golang.org/genproto/googleapis/rpc@v0.0.0-20261026165753-2ffab32bce7e \
        --replace google.golang.org/grpc=google.golang.org/grpc@v1.85.0 \
        --replace golang.org/x/crypto=golang.org/x/crypto@v0.58.0 \
        --replace golang.org/x/crypto/x509roots/fallback=golang.org/x/crypto/x509roots/fallback@v0.0.0-20261029161132-16804593e8b0 \
        --replace golang.org/x/net=golang.org/x/net@v0.62.0 \
        --replace golang.org/x/text=golang.org/x/text@v0.43.0 \
    && set -- /opt/caddy-build/buildenv_*/go.mod \
    && test "$#" -eq 1 \
    && cd "$(dirname "$1")" \
    && go list -deps -tags nobadger,nomysql,nopgx -json . > /out/caddy-packages.json \
    && /usr/local/bin/security-evidence -caddy /usr/bin/caddy -platform /out/platform \
        -caddy-graph /out/caddy-packages.json -platform-graph /out/platform-packages.json \
        -patches /opt/caddy-patches -sources /opt/caddy-deps/modules.json -out /out/security

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk add --no-cache ca-certificates tzdata
COPY --from=caddy-builder /usr/bin/caddy /usr/bin/caddy
COPY --from=builder /out/platform /usr/local/bin/platform
COPY --from=caddy-builder /out/security/evidence.json /out/security/openpgp.vex.json /usr/share/gateway/security/
RUN caddy version | grep -q '^v2[.]11[.]7 ' \
    && caddy list-modules | grep -qx 'dns.providers.azure' \
    && mkdir -p /data/platform /data/caddy
EXPOSE 80 443 8080
ENV GATEWAY_ROUTES_FILE=/data/platform/routes.json \
    GATEWAY_CERTIFICATE_FILE=/data/platform/certificate.json \
    XDG_DATA_HOME=/data/caddy
ENTRYPOINT ["/usr/local/bin/platform"]

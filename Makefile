SHELL := /bin/sh

IMAGE ?= pczhao1210/caddy-reverse-proxy:latest
SOCKET_PROXY_IMAGE ?= caddy-reverse-proxy-socket-proxy:0.5.0-haproxy3.4.7
HTTPBIN_IMAGE ?= caddy-reverse-proxy-httpbin:2.26.0-go1.27.3
BACKEND_DIR := backend
ENV_FILE ?= .env

.PHONY: help test test-e2e docker-build docker-build-companions docker-push docker-run compose-up compose-up-proxy compose-prod-up compose-prod-down compose-down azure-vm-deploy container-deploy

help:
	@printf '%s\n' 'Targets:'
	@printf '%s\n' '  test          Run Go tests in a Go toolchain container'
	@printf '%s\n' '  docker-build  Build the single platform gateway image'
	@printf '%s\n' '  docker-build-companions Build patched socket proxy and sample httpbin locally'
	@printf '%s\n' '  docker-push   Build, verify, and publish AMD64 + ARM64 for IMAGE'
	@printf '%s\n' '  docker-run    Run the image locally with ENV_FILE on Docker bridge'
	@printf '%s\n' '  compose-up    Start the VM profile sample stack'
	@printf '%s\n' '  compose-up-proxy Start the VM stack through a Docker socket proxy'
	@printf '%s\n' '  compose-prod-up Start the production VM gateway stack'
	@printf '%s\n' '  compose-prod-down Stop the production VM gateway stack'
	@printf '%s\n' '  azure-vm-deploy Interactively deploy a standalone Azure VM gateway'
	@printf '%s\n' '  container-deploy Deploy only the gateway container on this machine'
	@printf '%s\n' '  test-e2e      Exercise Caddy routing with the sample VM stack'
	@printf '%s\n' '  compose-down  Stop the VM profile sample stack'

test:
	docker run --rm -v "$$(pwd)/$(BACKEND_DIR):/src" -w /src golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb go test ./...

test-e2e:
	ENV_FILE=$(ENV_FILE) ./scripts/e2e-caddy-routing.sh

docker-build:
	docker build -f backend/Dockerfile -t $(IMAGE) .

docker-build-companions:
	docker build -f deploy/vm/Dockerfile.socket-proxy -t $(SOCKET_PROXY_IMAGE) .
	docker build -f deploy/vm/Dockerfile.httpbin -t $(HTTPBIN_IMAGE) .

docker-push:
	sh scripts/publish-multiarch.sh "$(IMAGE)"

docker-run:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE). Create one first, for example: cp .env.example .env' >&2; exit 1; }
	docker run --rm --env-file $(ENV_FILE) -p 8080:8080 -p 8081:80 \
		-v "$$(pwd)/config:/config:ro" \
		-v gateway-data:/data \
		-v /var/run/docker.sock:/var/run/docker.sock:ro \
		$(IMAGE)

compose-up:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE). Create one first, for example: cp .env.example .env' >&2; exit 1; }
	IMAGE=$(IMAGE) docker compose --env-file $(ENV_FILE) -f deploy/vm/docker-compose.yml up --build

compose-up-proxy:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE). Create one first, for example: cp .env.example .env' >&2; exit 1; }
	IMAGE=$(IMAGE) docker compose --env-file $(ENV_FILE) -f deploy/vm/docker-compose.socket-proxy.yml up --build

compose-prod-up:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE). Create one first, for example: cp .env.example .env' >&2; exit 1; }
	@token="$$(sed -n 's/^GATEWAY_ADMIN_TOKEN=//p' $(ENV_FILE) | tail -n 1)"; \
	if test -z "$$token" || test "$$token" = 'change-me'; then \
		printf '%s\n' 'Set GATEWAY_ADMIN_TOKEN to a strong random value before production startup.' >&2; \
		exit 1; \
	fi
	GATEWAY_ENV_FILE=$(abspath $(ENV_FILE)) IMAGE=$(IMAGE) docker compose --env-file $(ENV_FILE) -f deploy/vm/docker-compose.production.yml up -d --wait --wait-timeout 120

compose-prod-down:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE).' >&2; exit 1; }
	GATEWAY_ENV_FILE=$(abspath $(ENV_FILE)) IMAGE=$(IMAGE) docker compose --env-file $(ENV_FILE) -f deploy/vm/docker-compose.production.yml down

compose-down:
	@test -f $(ENV_FILE) || { printf '%s\n' 'Missing $(ENV_FILE). Create one first, for example: cp .env.example .env' >&2; exit 1; }
	IMAGE=$(IMAGE) docker compose --env-file $(ENV_FILE) -f deploy/vm/docker-compose.yml down

azure-vm-deploy:
	DEPLOY_MODE=azure-vm bash deploy/vm/deploy.sh

container-deploy:
	DEPLOY_MODE=local bash deploy/vm/deploy.sh

SHELL := /bin/sh

IMAGE ?= pczhao1210/caddy-reverse-proxy:latest
BACKEND_DIR := backend
ENV_FILE ?= .env

.PHONY: help test test-e2e docker-build docker-push docker-run compose-up compose-up-proxy compose-prod-up compose-prod-down compose-down azure-vm-deploy container-deploy

help:
	@printf '%s\n' 'Targets:'
	@printf '%s\n' '  test          Run Go tests in a Go toolchain container'
	@printf '%s\n' '  docker-build  Build the single platform gateway image'
	@printf '%s\n' '  docker-push   Push IMAGE after checking Docker login state'
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
	docker run --rm -v "$$(pwd)/$(BACKEND_DIR):/src" -w /src golang:1.25-alpine go test ./...

test-e2e:
	ENV_FILE=$(ENV_FILE) ./scripts/e2e-caddy-routing.sh

docker-build:
	docker build -f backend/Dockerfile -t $(IMAGE) .

docker-push:
	@docker info >/dev/null 2>&1 || { printf '%s\n' 'Docker daemon is not reachable. Start Docker first.' >&2; exit 1; }
	@config="$${DOCKER_CONFIG:-$$HOME/.docker}/config.json"; \
	if ! test -f "$$config" || ! grep -Eq '"auths"|"credsStore"|"credHelpers"' "$$config"; then \
		printf '%s\n' 'Docker does not look logged in. Run: docker login' >&2; \
		exit 1; \
	fi
	docker push $(IMAGE)

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

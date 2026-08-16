# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=whatomate
BINARY_PATH=./cmd/whatomate
# Must match .github/workflows/test.yml; .golangci.yml is written for v2.
GOLANGCI_VERSION=v2.11.4
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# The config used by every development target.
#
# dev/config.toml is checked into git and contains no secrets, so a fresh clone
# runs with nothing copied and nothing edited. config.toml (gitignored) is for
# real credentials — point at it with `make run CONFIG=config.toml`.
CONFIG ?= dev/config.toml

DEV_COMPOSE = docker compose -f dev/docker-compose.yml

# Dev ports. The frontend dev server (FRONTEND_PORT) proxies /api and /ws to
# the backend (BACKEND_PORT) — see frontend/ports.ts for why the two URLs are
# not interchangeable. Both are exported so vite and playwright read the same
# values, and BACKEND_PORT is handed to the server as WHATOMATE_SERVER__PORT so
# overriding it actually moves the backend.
BACKEND_PORT ?= 8080
FRONTEND_PORT ?= 3000
export BACKEND_PORT
export FRONTEND_PORT

# Docker parameters (production stack)
DOCKER_COMPOSE=docker compose -f docker-compose.yml

# --- Incremental build inputs -----------------------------------------------
# Real prerequisites, so `make build` twice in a row does nothing the second
# time and `make dev` doesn't reinstall node_modules on every run.
GO_SRC := $(shell find cmd internal pkg -type f -name '*.go' 2>/dev/null)
FRONTEND_SRC := $(shell find frontend/src frontend/public -type f 2>/dev/null)
FRONTEND_DEPS = \
	frontend/node_modules \
	frontend/package.json \
	frontend/index.html \
	frontend/vite.config.ts \
	frontend/ports.ts \
	$(FRONTEND_SRC)

all: build

# ============================================================================
# Build
# ============================================================================

.PHONY: build
build: $(BINARY_NAME)

# Development binary — no embedded frontend, so it runs in API-only mode unless
# app.frontend_dir points it at a build on disk (dev/config.toml does).
$(BINARY_NAME): $(GO_SRC) go.mod go.sum
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)

# Production binary with the frontend embedded. Always rebuilds: this is the
# shipped artifact and it should never be a cached surprise. Note it overwrites
# the same ./whatomate that `build` produces.
.PHONY: build-prod dist
build-prod: embed-frontend
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(BINARY_PATH)
	@echo "Production binary built: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@ls -lh $(BINARY_NAME)

# ListMonk-compatible alias.
dist: build-prod

# Copy the frontend build into the directory that //go:embed reads.
.PHONY: embed-frontend
embed-frontend: internal/frontend/dist/index.html

internal/frontend/dist/index.html: frontend/dist
	@echo "Copying frontend build to embed directory..."
	@rm -rf internal/frontend/dist/*
	@cp -r frontend/dist/* internal/frontend/dist/
	@echo "Frontend embedded successfully"

# ============================================================================
# Frontend
# ============================================================================

.PHONY: frontend-install frontend-build frontend-dev frontend-preview
frontend-install: frontend/node_modules

# touch -c so the directory mtime advances past package.json even when npm
# decides nothing changed; otherwise this reruns on every make.
frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm install
	@touch -c frontend/node_modules

frontend-build: frontend/dist

frontend/dist: $(FRONTEND_DEPS)
	cd frontend && npm run build
	@touch -c frontend/dist

frontend-dev: frontend/node_modules
	cd frontend && npm run dev

frontend-preview: frontend/node_modules
	cd frontend && npm run preview

# ============================================================================
# Development
# ============================================================================

# The one command to use for local work. Open http://localhost:$(FRONTEND_PORT).
#
# Builds a real binary first rather than `go run`: $$! then points at the
# server itself, so the trap can actually kill it. `go run` spawns the server
# as a child and exits leave it holding the port — which is how you end up with
# a phantom listener on :$(BACKEND_PORT).
.PHONY: dev dev-backend dev-frontend
dev: $(BINARY_NAME) frontend/node_modules
	@WHATOMATE_SERVER__PORT=$(BACKEND_PORT) FRONTEND_PORT=$(FRONTEND_PORT) ./$(BINARY_NAME) server -config $(CONFIG) -migrate

# Backend only, for when the frontend is already running elsewhere.
dev-backend: run-migrate

# Frontend only. Expects a backend on :$(BACKEND_PORT); without one the dev
# server answers /api with a 503 explaining that.
dev-frontend: frontend-dev

# Postgres + Redis in Docker, nothing else — the app runs natively against
# them. This is the normal way to get a database for `make dev`.
.PHONY: dev-infra dev-infra-down dev-infra-logs
dev-infra:
	$(DEV_COMPOSE) up -d
	@echo "Waiting for Postgres and Redis to report healthy..."
	@for i in $$(seq 1 60); do \
		if [ "$$($(DEV_COMPOSE) ps --format '{{.Health}}' db redis | sort -u)" = "healthy" ]; then \
			echo "  db and redis are ready."; exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo "  Timed out. Check '$(DEV_COMPOSE) logs'."; exit 1

dev-infra-down:
	$(DEV_COMPOSE) down

dev-infra-logs:
	$(DEV_COMPOSE) logs -f

# The whole dev stack in containers — backend and Vite too. For reproducing a
# clean machine, or working without Go/Node installed locally.
.PHONY: dev-docker rm-dev-docker
dev-docker:
	$(DEV_COMPOSE) --profile full up

# Tear the dev stack down including its volumes (throws away the database).
rm-dev-docker:
	$(DEV_COMPOSE) --profile full down -v

# ============================================================================
# Install / run
# ============================================================================

# Take an empty database to a working app: migrate, create the default admin,
# backfill legacy chatbot flows. Idempotent — safe to re-run.
.PHONY: install seed
install:
	$(GOCMD) run $(BINARY_PATH) install -config $(CONFIG) $(ARGS)

# ...and add demo contacts, tags, and a starter chatbot flow to look at.
seed:
	$(GOCMD) run $(BINARY_PATH) install -config $(CONFIG) -idempotent -yes -seed

.PHONY: run run-migrate migrate
run:
	$(GOCMD) run $(BINARY_PATH) server -config $(CONFIG)

run-migrate:
	$(GOCMD) run $(BINARY_PATH) server -config $(CONFIG) -migrate

migrate: run-migrate

# ============================================================================
# Testing
# ============================================================================

# Run tests. Uses gotestsum when available for live progress + a clear
# failure summary at the end. Falls back to the built-in `go test -v` so
# nothing breaks for devs who haven't installed it.
# Install:  go install gotest.tools/gotestsum@latest
# check-test-infra warns when the databases the suite needs are absent. Without
# them nearly every test skips and `go test` still prints ok, so a green run
# means nothing. Warns rather than fails: running the handful of pure-unit
# tests is still legitimate.
.PHONY: check-test-infra
check-test-infra:
	@if [ -z "$$TEST_DATABASE_URL" ] || [ -z "$$TEST_REDIS_URL" ]; then \
		echo ""; \
		echo "  ####################################################################"; \
		echo "  #  WARNING: TEST_DATABASE_URL / TEST_REDIS_URL are not both set.    #"; \
		echo "  #  Infrastructure-backed tests will SKIP, and a PASS proves little. #"; \
		echo "  ####################################################################"; \
		echo ""; \
		echo "  TEST_DATABASE_URL=\"host=localhost port=5432 user=whatomate \\"; \
		echo "    password=whatomate dbname=whatomate_test sslmode=disable\" \\"; \
		echo "  TEST_REDIS_URL=\"redis://localhost:6379/1\" make test"; \
		echo ""; \
	fi

.PHONY: test test-coverage
test: check-test-infra
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --hide-summary=skipped -- ./...; \
	else \
		echo "(install gotestsum for nicer output: go install gotest.tools/gotestsum@latest)"; \
		$(GOTEST) -v ./...; \
	fi

# Run tests with coverage. Same gotestsum fallback as `make test`.
test-coverage: check-test-infra
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --hide-summary=skipped -- -coverprofile=coverage.out ./...; \
	else \
		echo "(install gotestsum for nicer output: go install gotest.tools/gotestsum@latest)"; \
		$(GOTEST) -v -coverprofile=coverage.out ./...; \
	fi
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# E2E against the dev server: live frontend source, API proxied to the backend.
# This is the one to use while working on the frontend. Playwright starts vite
# itself (reusing yours if it's already up); the backend must be running.
.PHONY: test-e2e test-e2e-embedded
test-e2e:
	cd frontend && npm run test:e2e

# E2E against the frontend embedded in the built binary — what CI does, and the
# only way to catch embed/build-only breakage. Rebuilds first so the snapshot
# matches the working tree, then runs the binary under test itself.
#
# WHATOMATE_APP__FRONTEND_DIR= is load-bearing: $(CONFIG) sets frontend_dir for
# normal development, and that takes priority over the embedded copy. Clearing
# it here is what makes this actually test the embed rather than frontend/dist.
test-e2e-embedded: build-prod
	@if curl -sf http://localhost:$(BACKEND_PORT)/health >/dev/null 2>&1; then \
		echo "Something is already serving :$(BACKEND_PORT) — stop it first, or this would"; \
		echo "test that server instead of the binary just built."; exit 1; \
	fi
	@WHATOMATE_APP__FRONTEND_DIR= ./$(BINARY_NAME) server -config $(CONFIG) -migrate & \
	BACKEND_PID=$$!; \
	trap 'kill $$BACKEND_PID 2>/dev/null || true' INT TERM EXIT; \
	echo "Waiting for the built binary on :$(BACKEND_PORT) ..."; \
	for i in $$(seq 1 60); do \
		if curl -sf http://localhost:$(BACKEND_PORT)/health >/dev/null 2>&1; then break; fi; \
		if ! kill -0 $$BACKEND_PID 2>/dev/null; then echo "Binary exited during startup."; exit 1; fi; \
		sleep 1; \
	done; \
	cd frontend && npm run test:embedded

# ============================================================================
# Docker (production stack)
# ============================================================================

.PHONY: docker-build docker-up docker-down docker-logs docker-restart
docker-build:
	$(DOCKER_COMPOSE) build

docker-up:
	$(DOCKER_COMPOSE) up -d

docker-down:
	$(DOCKER_COMPOSE) down

docker-logs:
	$(DOCKER_COMPOSE) logs -f

docker-restart:
	$(DOCKER_COMPOSE) restart

# ============================================================================
# Housekeeping
# ============================================================================

.PHONY: clean deps deps-update lint fmt
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

deps:
	$(GOMOD) download
	$(GOMOD) tidy

deps-update:
	$(GOMOD) tidy
	$(GOGET) -u ./...

lint:
	@golangci-lint version 2>/dev/null | grep -q "version 2\." || { \
		echo "golangci-lint v2 required (CI pins $(GOLANGCI_VERSION)); found:"; \
		golangci-lint version 2>/dev/null || echo "  not installed"; \
		echo "install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)"; \
		exit 1; }
	golangci-lint run ./...

fmt:
	$(GOCMD) fmt ./...

.PHONY: help
help:
	@echo "Getting started (from a fresh clone):"
	@echo "  make dev-infra   - Start Postgres + Redis in Docker"
	@echo "  make install     - Create the schema and the default admin"
	@echo "  make dev         - Run the app, then open http://localhost:$(FRONTEND_PORT)"
	@echo ""
	@echo "  No config to copy or edit: dev targets use $(CONFIG)."
	@echo "  Login with admin@admin.com / admin. Add 'make seed' for demo data."
	@echo ""
	@echo "Development:"
	@echo "  dev            - Backend + frontend together (the normal one)"
	@echo "  dev-backend    - Backend only, on :$(BACKEND_PORT)"
	@echo "  dev-frontend   - Frontend only, on :$(FRONTEND_PORT) (needs a backend)"
	@echo "  dev-infra      - Postgres + Redis only, app runs natively"
	@echo "  dev-infra-down - Stop them (keeps the data)"
	@echo "  dev-docker     - Whole stack in containers, backend and Vite included"
	@echo "  rm-dev-docker  - Tear that down and delete its volumes"
	@echo ""
	@echo "  Open :$(FRONTEND_PORT) while developing — it serves the frontend from"
	@echo "  source. :$(BACKEND_PORT) serves the last 'make frontend-build'."
	@echo ""
	@echo "Database:"
	@echo "  install        - Migrate + default admin (idempotent). ARGS=-seed to add demo data"
	@echo "  seed           - Add demo contacts, tags and a starter chatbot flow"
	@echo ""
	@echo "Build:"
	@echo "  build          - Development binary (no embedded frontend)"
	@echo "  build-prod     - Single binary with the frontend embedded ('dist' also works)"
	@echo "  frontend-build - Build the frontend into frontend/dist"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run Go tests"
	@echo "  test-coverage  - Run Go tests with coverage report"
	@echo "  test-e2e       - Playwright against the dev server (:$(FRONTEND_PORT))"
	@echo "  test-e2e-embedded - Playwright against the built binary (:$(BACKEND_PORT))"
	@echo ""
	@echo "Docker (production stack):"
	@echo "  docker-build / docker-up / docker-down / docker-logs"
	@echo ""
	@echo "Other:"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download dependencies"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"

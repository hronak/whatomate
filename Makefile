.PHONY: all build build-prod run test test-e2e test-e2e-embedded clean docker-build docker-up docker-down migrate dev dev-backend dev-frontend frontend-dev frontend-build

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

# Dev ports. The frontend dev server (FRONTEND_PORT) proxies /api and /ws to
# the backend (BACKEND_PORT) — see frontend/ports.ts for why the two URLs are
# not interchangeable. Both are exported so vite and playwright read the same
# values, and BACKEND_PORT is handed to the server as WHATOMATE_SERVER__PORT so
# overriding it actually moves the backend.
BACKEND_PORT ?= 8080
FRONTEND_PORT ?= 3000
export BACKEND_PORT
export FRONTEND_PORT

# Docker parameters
DOCKER_COMPOSE=docker compose -f docker-compose.yml

all: build

# Build the backend (development - without frontend)
build:
	$(GOBUILD) -o $(BINARY_NAME) $(BINARY_PATH)

# Build production binary with embedded frontend
build-prod: frontend-build embed-frontend
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(BINARY_PATH)
	@echo "Production binary built: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@ls -lh $(BINARY_NAME)

# Copy frontend build to embed directory
embed-frontend:
	@echo "Copying frontend build to embed directory..."
	@rm -rf internal/frontend/dist/*
	@cp -r frontend/dist/* internal/frontend/dist/
	@echo "Frontend embedded successfully"

# Run the backend locally
run:
	$(GOCMD) run $(BINARY_PATH)/main.go server -config config.toml

# Run with migrations
run-migrate:
	$(GOCMD) run $(BINARY_PATH)/main.go server -config config.toml -migrate

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
test-e2e:
	cd frontend && npm run test:e2e

# E2E against the frontend embedded in the built binary — what CI does, and the
# only way to catch embed/build-only breakage. Rebuilds first so the snapshot
# matches the working tree, then runs the binary under test itself.
test-e2e-embedded: build-prod
	@if curl -sf http://localhost:$(BACKEND_PORT)/health >/dev/null 2>&1; then \
		echo "Something is already serving :$(BACKEND_PORT) — stop it first, or this would"; \
		echo "test that server instead of the binary just built."; exit 1; \
	fi
	@./$(BINARY_NAME) server -config config.toml -migrate & \
	BACKEND_PID=$$!; \
	trap 'kill $$BACKEND_PID 2>/dev/null || true' INT TERM EXIT; \
	echo "Waiting for the built binary on :$(BACKEND_PORT) ..."; \
	for i in $$(seq 1 60); do \
		if curl -sf http://localhost:$(BACKEND_PORT)/health >/dev/null 2>&1; then break; fi; \
		if ! kill -0 $$BACKEND_PID 2>/dev/null; then echo "Binary exited during startup."; exit 1; fi; \
		sleep 1; \
	done; \
	cd frontend && npm run test:embedded

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Update dependencies
deps-update:
	$(GOMOD) tidy
	$(GOGET) -u ./...

# Docker commands
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

# Database migrations
migrate:
	$(GOCMD) run $(BINARY_PATH)/main.go server -config config.toml -migrate

# Frontend commands
frontend-install:
	cd frontend && npm install

frontend-dev:
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	cd frontend && npm run dev

frontend-build:
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	cd frontend && npm run build

frontend-preview:
	cd frontend && npm run preview

# Development - run both backend and frontend.
#
# The one command to use for local work. Open http://localhost:$(FRONTEND_PORT)
# — never the backend port, which serves the frontend snapshot embedded at the
# last `make build-prod` and so will not show your frontend edits.
#
# Builds a real binary first rather than `go run`: $$! then points at the
# server itself, so the trap can actually kill it. `go run` spawns the server
# as a child and exits leave it holding the port — which is how you end up with
# a phantom listener on :$(BACKEND_PORT).
dev: build
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	@BINARY=./$(BINARY_NAME) bash ./scripts/dev.sh

# Backend only, for when the frontend is already running elsewhere.
dev-backend: run-migrate

# Frontend only. Expects a backend on :$(BACKEND_PORT); without one the dev
# server answers /api with a 503 explaining that.
dev-frontend: frontend-dev

# Lint
lint:
	@golangci-lint version 2>/dev/null | grep -q "version 2\." || { \
		echo "golangci-lint v2 required (CI pins $(GOLANGCI_VERSION)); found:"; \
		golangci-lint version 2>/dev/null || echo "  not installed"; \
		echo "install: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)"; \
		exit 1; }
	golangci-lint run ./...

# Format code
fmt:
	$(GOCMD) fmt ./...

# Generate swagger docs (if using)
swagger:
	swag init -g cmd/whatomate/main.go -o api/docs

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Production:"
	@echo "  build-prod     - Build single binary with embedded frontend"
	@echo ""
	@echo "Development:"
	@echo "  dev            - Run backend + frontend, then open http://localhost:$(FRONTEND_PORT)"
	@echo "  dev-backend    - Backend only, on :$(BACKEND_PORT) (alias of run-migrate)"
	@echo "  dev-frontend   - Frontend only, on :$(FRONTEND_PORT) (needs a backend)"
	@echo "  build          - Build the backend binary (without frontend)"
	@echo "  run            - Run the backend locally"
	@echo "  run-migrate    - Run the backend with database migrations"
	@echo ""
	@echo "  Open :$(FRONTEND_PORT) while developing. :$(BACKEND_PORT) serves the frontend"
	@echo "  snapshot embedded at the last 'make build-prod', not your edits."
	@echo ""
	@echo "Frontend:"
	@echo "  frontend-install - Install frontend dependencies"
	@echo "  frontend-dev   - Run frontend in development mode"
	@echo "  frontend-build - Build frontend for production"
	@echo ""
	@echo "Testing:"
	@echo "  test           - Run Go tests"
	@echo "  test-coverage  - Run Go tests with coverage report"
	@echo "  test-e2e       - Run Playwright e2e against the dev server (:$(FRONTEND_PORT))"
	@echo "  test-e2e-embedded - Run Playwright against the built binary (:$(BACKEND_PORT))"
	@echo ""
	@echo "Docker:"
	@echo "  docker-build   - Build Docker images"
	@echo "  docker-up      - Start Docker containers"
	@echo "  docker-down    - Stop Docker containers"
	@echo "  docker-logs    - View Docker logs"
	@echo ""
	@echo "Other:"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download dependencies"
	@echo "  lint           - Run linter"
	@echo "  fmt            - Format code"

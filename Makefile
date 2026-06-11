APP_NAME := ai-proxy-gateway
MAIN_PKG := ./cmd/gateway
BUILD_DIR := bin
CONFIG ?= configs/config.example.json
IMAGE ?= ai-proxy-gateway:latest
DOCKERFILE ?= docker/dockerfile
COMPOSE_FILE ?= deploy/docker-compose.yml

GO ?= go
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0
LDFLAGS := -s -w

.PHONY: help tidy fmt test build run clean docker-build compose-up compose-down compose-logs

help:
	@echo "Targets:"
	@echo "  make tidy          - tidy go modules"
	@echo "  make fmt           - format Go code"
	@echo "  make test          - run tests"
	@echo "  make build         - build binary into $(BUILD_DIR)/$(APP_NAME)"
	@echo "  make run           - run gateway with CONFIG=$(CONFIG)"
	@echo "  make clean         - remove build artifacts"
	@echo "  make docker-build  - build Docker image IMAGE=$(IMAGE)"
	@echo "  make compose-up    - start docker compose stack"
	@echo "  make compose-down  - stop docker compose stack"
	@echo "  make compose-logs  - follow docker compose logs"

tidy:
	$(GO) mod tidy

fmt:
	gofmt -w cmd internal

test:
	$(GO) test ./...

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

run:
	$(GO) run $(MAIN_PKG) -config $(CONFIG)

clean:
	rm -rf $(BUILD_DIR)

docker-build:
	docker build -f $(DOCKERFILE) -t $(IMAGE) .

compose-up:
	docker compose -f $(COMPOSE_FILE) up -d --build

compose-down:
	docker compose -f $(COMPOSE_FILE) down

compose-logs:
	docker compose -f $(COMPOSE_FILE) logs -f

GO ?= go
GO_VERSION ?= 1.23
GO_IMAGE ?= golang:$(GO_VERSION)
DOCKER_GO_FLAGS ?=
DOCKER_GO_RUN = docker run --rm -v $(CURDIR):/src -w /src $(DOCKER_GO_FLAGS) $(GO_IMAGE)

HAVE_GO := $(shell command -v go >/dev/null 2>&1 && echo 1 || echo 0)
ifeq ($(HAVE_GO),1)
USE_DOCKER_GO ?= 0
else
USE_DOCKER_GO ?= 1
endif

ifeq ($(USE_DOCKER_GO),1)
GO_CMD := $(DOCKER_GO_RUN) go
else
GO_CMD := $(GO)
endif

GOOS ?= $(shell $(GO_CMD) env GOOS 2>/dev/null || echo linux)
GOARCH ?= $(shell $(GO_CMD) env GOARCH 2>/dev/null || echo amd64)
CGO_ENABLED ?= 0
BINARY ?= backrest-sidecar
BIN_DIR ?= bin
CMD_PATH ?= ./cmd/backrest-sidecar
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
GO_BUILD_FLAGS ?= -buildvcs=false
TAG ?= backrest-sidecar:dev
CONFIG ?= ./backrest.config.json
RUN_FLAGS ?=
DOCKER_BUILD_ARGS ?=
DOCKER_ARGS ?= daemon --config /etc/backrest/config.json --with-events --apply
COMPOSE_FILE ?= testing/compose.dry-run.yaml
COMPOSE_PROJECT ?= backrest-sidecar
COMPOSE_CMD ?= ps

BIN := $(BIN_DIR)/$(BINARY)

.PHONY: build run dry-run docker-build docker-run docker-push fmt test clean compose

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO_CMD) build $(GO_BUILD_FLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD_PATH)

run: build
	$(BIN) $(ARGS)

dry-run: build
	@if [ ! -f $(CONFIG) ]; then echo "CONFIG '$(CONFIG)' not found; set CONFIG=/path/to/config.json"; exit 1; fi
	$(BIN) reconcile --dry-run --config $(CONFIG) $(RUN_FLAGS)

docker-build:
	docker build $(DOCKER_BUILD_ARGS) -t $(TAG) .

docker-run: docker-build
	docker run --rm \
		-v $(CONFIG):/etc/backrest/config.json \
		-v /var/run/docker.sock:/var/run/docker.sock:ro \
		-v /var/lib/docker:/var/lib/docker:ro \
		$(TAG) $(DOCKER_ARGS)

docker-push:
	docker push $(TAG)

fmt:
	$(GO_CMD) fmt ./...

test:
	$(GO_CMD) test ./...

compose:
	COMPOSE_PROJECT_NAME=$(COMPOSE_PROJECT) docker compose -p $(COMPOSE_PROJECT) -f $(COMPOSE_FILE) $(COMPOSE_CMD)

clean:
	rm -rf $(BIN_DIR)

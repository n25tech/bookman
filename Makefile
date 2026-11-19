# bookman: see README.md
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE    ?= registry.home/bookman/bookman
GO       ?= go
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build web test test-e2e dev dev-web image push clean

all: build

web/node_modules: web/package-lock.json
	cd web && npm ci
	@touch $@

web: web/node_modules
	cd web && npm run build

## build: the single static binary at bin/bookman (UI embedded)
build: web
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/bookman ./cmd/bookman

## test: Go tests (+ the API integration test when BOOKMAN_TEST_DATABASE_URL is set) and the frontend type check
test: web/node_modules
	CGO_ENABLED=0 $(GO) vet ./...
	CGO_ENABLED=0 $(GO) test ./...
	cd web && npm run check

## test-e2e: Playwright against a running server (BOOKMAN_E2E_URL, BOOKMAN_E2E_USER, BOOKMAN_E2E_PASSWORD, BOOKMAN_E2E_BOOK)
test-e2e: web/node_modules
	cd web && npx playwright test

## dev: the Go server on :8080; run `make dev-web` alongside for the Vite dev server on :5173
dev:
	$(GO) run ./cmd/bookman -config $${BOOKMAN_CONFIG:-bookman.dev.toml} -log-level debug

dev-web: web/node_modules
	cd web && npm run dev

## image: build the container image with docker (or podman: make image DOCKER=podman)
DOCKER ?= docker
image:
	$(DOCKER) build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

push: image
	$(DOCKER) push $(IMAGE):$(VERSION)
	$(DOCKER) push $(IMAGE):latest

clean:
	rm -f bin/bookman

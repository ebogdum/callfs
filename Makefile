MODULE := github.com/ebogdum/callfs
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(MODULE)/internal/buildinfo.Version=$(VERSION) -X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) -X $(MODULE)/internal/buildinfo.Date=$(DATE)

.PHONY: web-install build-ui build build-noui test test-ui

web-install:
	cd web && npm ci

build-ui:
	cd web && npm run build

build:
	go build -tags ui -ldflags "$(LDFLAGS)" -o callfs ./cmd

build-noui:
	go build -ldflags "$(LDFLAGS)" -o callfs ./cmd

test:
	go test ./...

test-ui:
	go test -tags ui ./...

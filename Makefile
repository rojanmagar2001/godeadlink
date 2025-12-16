
# =========================
# Project configuration
# =========================

APP_NAME      := deadlink
CMD_DIR       := ./cmd/deadlink
BIN_DIR       := ./bin
GO            := go
GOFLAGS       := -trimpath
LDFLAGS       := -s -w

# Default URL for quick testing (override on command line)
URL           ?= https://example.com
TIMEOUT       ?= 10s
HEAD_FIRST    ?= true
CONCURRENCY ?= 20

# =========================
# Phony targets
# =========================

.PHONY: help tidy build run test test-race clean fmt vet lint check dev

# =========================
# Help
# =========================

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  tidy        - Run go mod tidy"
	@echo "  build       - Build binary into ./bin/"
	@echo "  run         - Run app against a URL"
	@echo "  test        - Run unit tests"
	@echo "  test-race   - Run tests with race detector"
	@echo "  fmt         - Format code"
	@echo "  vet         - Run go vet"
	@echo "  lint        - Run golangci-lint (if installed)"
	@echo "  check       - fmt + vet + test"
	@echo "  dev         - fmt, test, then run"
	@echo "  clean       - Remove build artifacts"
	@echo ""
	@echo "Variables:"
	@echo "  URL=https://example.com"
	@echo "  TIMEOUT=10s"
	@echo "  HEAD_FIRST=true"

# =========================
# Dependency management
# =========================

tidy:
	$(GO) mod tidy

# =========================
# Build & Run
# =========================

build:
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

run:
	$(GO) run $(CMD_DIR) \
		--url $(URL) \
		--timeout $(TIMEOUT) \
		--head-first=$(HEAD_FIRST) \
		--concurrency=$(CONCURRENCY)

dev: fmt test run

# =========================
# Testing
# =========================

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

# =========================
# Code quality
# =========================

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Skipping."; \
		echo "Install from https://golangci-lint.run/"; \
	fi

check: fmt vet test

# =========================
# Cleanup
# =========================

clean:
	rm -rf $(BIN_DIR)

.PHONY: bench

bench:
	$(GO) test ./... -bench=. -benchmem

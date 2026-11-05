# sing-box-panel Makefile for development, checks and releases

WEB_PNPM       := cd web && corepack pnpm
PROTO_DIR      := api/proto
RELEASE_SCRIPT := scripts/build.sh
GO_SOURCE_DIRS := cmd internal tools web/apps

.PHONY: help check check-backend check-frontend check-contracts test test-backend test-frontend \
	test-race fuzz-smoke core-contract ci build build-panel build-node build-frontend \
	fmt generate proto-check support-generate support-check notices \
	snapshot release release-verify release-smoke deps deps-backend deps-frontend clean

# Default target
help:
	@echo "sing-box-panel Development Commands"
	@echo ""
	@echo "Checks and testing:"
	@echo "  make check                       - Run every required check (backend + frontend + contracts)"
	@echo "  make check-backend               - Go formatting, modules, vet and tests"
	@echo "  make check-frontend              - Web lint, tests, Schema outputs and third-party notices"
	@echo "  make check-contracts             - Shell, installer, protobuf, support and OpenAPI contracts"
	@echo "  make test                        - Run Go and Web tests"
	@echo "  make test-backend                - Run Go tests only"
	@echo "  make test-frontend               - Run Web tests only"
	@echo "  make test-race                   - Run Go tests with the race detector"
	@echo "  make fuzz-smoke                  - Run short fuzzing passes"
	@echo "  make core-contract               - Check reviewed cores on native Linux"
	@echo "  make ci                          - check, test-race, fuzz-smoke and release-verify"
	@echo ""
	@echo "Build:"
	@echo "  make build                       - Build both binaries and the Web application"
	@echo "  make build-panel                 - Build bin/sing-box-panel"
	@echo "  make build-node                  - Build bin/sing-box-node"
	@echo "  make build-frontend              - Build the panel and node Web applications"
	@echo ""
	@echo "Development:"
	@echo "  make fmt                         - Format Go sources"
	@echo "  make generate                    - Regenerate the panel-node protocol code"
	@echo "  make proto-check                 - Lint the protocol and check generated code"
	@echo "  make support-generate            - Regenerate sing-box support code, Schemas and tables"
	@echo "  make support-check               - Check sing-box support outputs"
	@echo "  make notices                     - Regenerate THIRD_PARTY_NOTICES"
	@echo ""
	@echo "Release:"
	@echo "  make snapshot OUT=dir            - Build an unsigned snapshot into OUT"
	@echo "  make release OUT=dir VERSION=v   - Build a release into OUT"
	@echo "  make release-verify              - Check the release build scripts"
	@echo "  make release-smoke               - Check a packaged release on native Linux"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps                        - Install Go and Web dependencies"
	@echo "  make deps-backend                - Download Go modules"
	@echo "  make deps-frontend               - Install Web dependencies from the lockfile"
	@echo ""
	@echo "Clean:"
	@echo "  make clean                       - Remove build outputs and the Go test cache"

# =============================================================================
# Checks and testing
# =============================================================================

# Run every required check
check: check-backend check-frontend check-contracts
	@echo "✅ All checks passed"

# Go formatting, modules, vet and tests; the panel embeds the Web build
check-backend: build-frontend
	@echo "🔍 Checking Go sources..."
	@files="$$(gofmt -l $$(find $(GO_SOURCE_DIRS) -type f -name '*.go'))"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test ./...

# Web lint, tests, generated Schema outputs and third-party notices
check-frontend: build-frontend
	@echo "🔍 Checking Web sources..."
	$(WEB_PNPM) run lint
	$(WEB_PNPM) --recursive run test
	$(WEB_PNPM) run schema:check
	go tool third-party-notices --check

# Shell syntax, installer scenarios, protocol, sing-box support and OpenAPI
check-contracts: build-frontend support-check proto-check
	@echo "🔍 Checking contracts..."
	@for script in $$(find scripts -type f -name '*.sh'); do bash -n "$$script" || exit; done
	bash scripts/test/installer-test.sh
	go tool verify-openapi api/openapi.yaml
	go tool verify-openapi api/node-openapi.yaml

# Run Go and Web tests
test: test-backend test-frontend
	@echo "✅ All tests completed"

# Go tests only
test-backend: build-frontend
	@echo "🧪 Running Go tests..."
	go test ./...

# Web tests only
test-frontend:
	@echo "🧪 Running Web tests..."
	$(WEB_PNPM) test

# Go tests with the race detector
test-race: build-frontend
	@echo "🧪 Running Go tests with the race detector..."
	go test -race ./...

# Short fuzzing passes over version parsing and subscription rendering
fuzz-smoke: build-frontend
	@echo "🧪 Running fuzz smoke tests..."
	go test ./internal/coreartifact -run '^$$' -fuzz '^FuzzParseExactVersionCanonicalRoundTrip$$' -fuzztime=5s
	go test ./internal/sing-box-panel/subscription -run '^$$' -fuzz '^FuzzRenderIsPureAndDeterministic$$' -fuzztime=5s

# Reviewed official cores on native Linux amd64/arm64
core-contract: build-frontend
	@echo "🧪 Running the core contract..."
	bash scripts/test/core-contract.sh

# Everything CI runs on a pull request
ci: check test-race fuzz-smoke release-verify

# =============================================================================
# Build
# =============================================================================

# Build both binaries and the Web application
build: build-panel build-node

# Build the panel binary with the embedded Web application
build-panel: build-frontend
	@echo "🔨 Building panel..."
	@mkdir -p bin
	go build -trimpath -o bin/sing-box-panel ./cmd/sing-box-panel
	@echo "✅ Panel built: ./bin/sing-box-panel"

# Build the node binary
build-node:
	@echo "🔨 Building node..."
	@mkdir -p bin
	go build -trimpath -o bin/sing-box-node ./cmd/sing-box-node
	@echo "✅ Node built: ./bin/sing-box-node"

# Build the Web application
build-frontend:
	@echo "🔨 Building Web application..."
	$(WEB_PNPM) run build
	@echo "✅ Web applications built: ./web/apps/panel/dist and ./web/apps/node/dist"

# =============================================================================
# Development
# =============================================================================

# Format Go sources
fmt:
	@echo "🎨 Formatting Go sources..."
	gofmt -w $$(find $(GO_SOURCE_DIRS) -type f -name '*.go')

# Regenerate internal/nodeapi from the protobuf contract
generate:
	@echo "🔧 Generating protocol code..."
	go tool buf generate $(PROTO_DIR) --template $(PROTO_DIR)/buf.gen.yaml

# Lint the protocol and regenerate into a scratch directory, so drift fails
# without touching the tree
proto-check:
	@echo "🔍 Checking protocol code..."
	go tool buf lint $(PROTO_DIR)
	@out="$$(mktemp -d)" || exit; trap 'rm -rf -- "$$out"' EXIT; \
	go tool buf generate $(PROTO_DIR) --template $(PROTO_DIR)/buf.gen.yaml --output "$$out" || exit; \
	for file in "$$out"/internal/nodeapi/*.pb.go internal/nodeapi/*.pb.go; do \
		name="$${file##*/}"; \
		cmp -s "$$out/internal/nodeapi/$$name" "internal/nodeapi/$$name" || { printf '%s\n' "internal/nodeapi/$$name is stale; run make generate" >&2; exit 1; }; \
	done

# Regenerate sing-box support code, exact-version Schemas and the support table
support-generate:
	@echo "🔧 Generating sing-box support..."
	go tool singbox-support generate

# Check sing-box support outputs without rewriting them
support-check:
	@echo "🔍 Checking sing-box support..."
	go tool singbox-support check

# Regenerate THIRD_PARTY_NOTICES from both binaries and the Web build
notices: build-frontend
	@echo "🔧 Generating third-party notices..."
	go tool third-party-notices

# =============================================================================
# Release
# =============================================================================

# Build an unsigned snapshot: make snapshot OUT=dir
snapshot:
	@test -n "$(OUT)" || { printf '%s\n' 'OUT is required' >&2; exit 2; }
	@echo "📦 Building snapshot..."
	$(RELEASE_SCRIPT) snapshot --output "$(OUT)"

# Build a release: make release OUT=dir VERSION=vX.Y.Z
release:
	@test -n "$(OUT)" || { printf '%s\n' 'OUT is required' >&2; exit 2; }
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required' >&2; exit 2; }
	@echo "📦 Building release $(VERSION)..."
	$(RELEASE_SCRIPT) release --version "$(VERSION)" --output "$(OUT)"

# Check the release build scripts
release-verify:
	@echo "🔍 Verifying release build..."
	$(RELEASE_SCRIPT) verify

# Check a packaged release on native Linux
release-smoke: build-frontend
	@echo "🧪 Running release smoke test..."
	bash scripts/test/release-contract.sh

# =============================================================================
# Dependencies
# =============================================================================

# Install Go and Web dependencies
deps: deps-backend deps-frontend

# Download Go modules
deps-backend:
	@echo "📦 Downloading Go modules..."
	go mod download
	@echo "✅ Go modules downloaded"

# Install Web dependencies exactly as locked
deps-frontend:
	@echo "📦 Installing Web dependencies..."
	$(WEB_PNPM) install --frozen-lockfile --ignore-scripts --verify-store-integrity
	@echo "✅ Web dependencies installed"

# =============================================================================
# Clean
# =============================================================================

clean:
	@echo "🧹 Cleaning..."
	rm -rf bin web/apps/panel/dist web/apps/node/dist
	go clean -testcache
	@echo "✅ Cleaned"

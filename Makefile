.DEFAULT_GOAL := build

WEB_PNPM := cd web && corepack pnpm
GO_SOURCE_DIRS := api cmd internal systemd web
RELEASE_SCRIPT := scripts/build.sh

.PHONY: \
	bootstrap build web-build fmt \
	check check-go check-web check-contracts test-race fuzz-smoke core-contract ci \
	notices support-generate support-check \
	snapshot release release-verify release-smoke

# Development and builds

bootstrap:
	go mod download
	$(WEB_PNPM) install --frozen-lockfile --ignore-scripts --verify-store-integrity

build: web-build
	mkdir -p bin
	go build -trimpath -o bin/sing-box-panel ./cmd/sing-box-panel

web-build:
	$(WEB_PNPM) run build

fmt:
	gofmt -w $$(find $(GO_SOURCE_DIRS) -type f -name '*.go')

# Checks and tests

check: check-go check-web check-contracts

check-go: web-build
	@files="$$(gofmt -l $$(find $(GO_SOURCE_DIRS) -type f -name '*.go'))"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi
	go mod tidy -diff
	go vet ./...
	go test ./...

check-web:
	bash scripts/test/check-web.sh

check-contracts: web-build support-check
	@for script in $$(find scripts -type f -name '*.sh'); do bash -n "$$script" || exit; done
	bash scripts/test/installer-test.sh
	go tool verify-openapi api/openapi.yaml

test-race: web-build
	go test -race ./...

fuzz-smoke: web-build
	go test ./internal/coreartifact -run '^$$' -fuzz '^FuzzParseExactVersionCanonicalRoundTrip$$' -fuzztime=5s
	go test ./internal/subscription -run '^$$' -fuzz '^FuzzRenderIsPureAndDeterministic$$' -fuzztime=5s

core-contract: web-build
	bash scripts/test/core-contract.sh

ci: check test-race fuzz-smoke release-verify

# Generation and maintenance

notices: web-build
	go tool third-party-notices

support-generate:
	go tool singbox-support generate

support-check:
	go tool singbox-support check

# Release

snapshot:
	@test -n "$(OUT)" || { printf '%s\n' 'OUT is required' >&2; exit 2; }
	$(RELEASE_SCRIPT) snapshot --output "$(OUT)"

release:
	@test -n "$(OUT)" || { printf '%s\n' 'OUT is required' >&2; exit 2; }
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required' >&2; exit 2; }
	$(RELEASE_SCRIPT) release --version "$(VERSION)" --output "$(OUT)"

release-smoke: web-build
	bash scripts/test/release-contract.sh

release-verify:
	$(RELEASE_SCRIPT) verify

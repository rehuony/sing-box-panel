WEB_PNPM := pnpm --dir web
GO_SOURCE_DIRS := cmd internal systemd web
RELEASE_SCRIPT := .github/scripts/build-release.sh
SHELL_SOURCE_DIRS := .github/scripts

.PHONY: \
	bootstrap go-download web-install \
	fmt notices web-lint-fix \
	fmt-check mod-check vet test test-race fuzz-smoke \
	web-lint web-test web-typecheck \
	notices-check openapi-check shell-check \
	check-go check-web check-contracts check \
	web-build build \
	require-out require-version release release-verify snapshot \
	ci

# Bootstrap

go-download:
	go mod download

web-install:
	corepack enable pnpm
	$(WEB_PNPM) install --frozen-lockfile --ignore-scripts --verify-store-integrity

bootstrap: go-download web-install

# Writable maintenance

fmt:
	gofmt -w $$(find $(GO_SOURCE_DIRS) -type f -name '*.go')

notices:
	go tool third-party-notices

web-lint-fix:
	$(WEB_PNPM) run lint:fix

# Read-only checks

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

fmt-check:
	@files="$$(gofmt -l $$(find $(GO_SOURCE_DIRS) -type f -name '*.go'))"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi

mod-check:
	go mod tidy -diff

fuzz-smoke:
	go test ./internal/coreartifact -run '^$$' -fuzz '^FuzzParseExactVersionCanonicalRoundTrip$$' -fuzztime=5s
	go test ./internal/subscription/render -run '^$$' -fuzz '^FuzzRenderIsPureAndDeterministic$$' -fuzztime=5s

web-lint web-test web-typecheck:
	$(WEB_PNPM) run $(patsubst web-%,%,$@)

shell-check:
	bash -n $$(find $(SHELL_SOURCE_DIRS) -type f -name '*.sh')

notices-check:
	go tool third-party-notices --check

openapi-check:
	go tool verify-openapi api/openapi.yaml

check-go: fmt-check mod-check vet test

check-web: web-lint web-typecheck web-test

check-contracts: shell-check openapi-check notices-check

check: check-go check-web check-contracts

# Local build

web-build: web-typecheck
	$(WEB_PNPM) run bundle

build: web-build
	mkdir -p bin
	go build -tags webdist -trimpath -o bin/sing-box-panel ./cmd/sing-box-panel

# Release

require-out:
	@test -n "$(OUT)" || { printf '%s\n' 'OUT is required' >&2; exit 2; }

require-version:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required' >&2; exit 2; }

release: require-out require-version
	$(RELEASE_SCRIPT) release --version "$(VERSION)" --output "$(OUT)"

release-verify:
	$(RELEASE_SCRIPT) verify

snapshot: require-out
	$(RELEASE_SCRIPT) snapshot --output "$(OUT)"

# Continuous integration

ci: check test-race fuzz-smoke release-verify

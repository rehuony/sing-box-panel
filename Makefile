.PHONY: generate notices notices-check openapi-check web-install web-lint web-test web-build fmt fmt-check test vet check build

generate:
	go generate ./...

notices:
	go run ./cmd/third-party-notices

notices-check:
	go run ./cmd/third-party-notices --check

openapi-check:
	go run ./cmd/verify-openapi api/openapi.yaml

web-install:
	cd web && pnpm install --frozen-lockfile --ignore-scripts --verify-store-integrity

web-lint:
	cd web && pnpm run lint

web-test:
	cd web && pnpm test

web-build:
	cd web && pnpm run build

fmt:
	gofmt -w $$(find cmd internal release web -name '*.go' -type f 2>/dev/null)

fmt-check:
	@files="$$(gofmt -l $$(find cmd internal release web -name '*.go' -type f 2>/dev/null))"; if [ -n "$$files" ]; then printf '%s\n' "$$files"; exit 1; fi

test:
	go test ./...

vet:
	go vet ./...

check: notices-check openapi-check web-lint web-test web-build fmt-check vet test

build: web-build
	go build -tags webdist -trimpath -o bin/sing-box-panel ./cmd/sing-box-panel

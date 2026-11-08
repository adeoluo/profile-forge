BINARY   := profile-forge
PKG      := github.com/adeoluo/profile-forge
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -buildid= -X $(PKG)/internal/cli.Version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64
DIST     := dist

.PHONY: all build test race cover vet fmt fmt-check lint bench example check-example release verify-release clean changelog cut-release

all: fmt-check vet race build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test ./...

race:
	go test -race -count=1 ./...

cover:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run make fmt" && exit 1)

lint: vet
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

bench:
	go test -run '^$$' -bench . -benchmem ./internal/render ./internal/validate

# Regenerate the committed example from live GitHub data.
example: build
	./bin/$(BINARY) generate --config examples/adeoluo.yaml --out examples/adeoluo.README.md --json examples/adeoluo.portfolio.json \
		--diagrams-dir assets --mermaid-config examples/mermaid.config.json

check-example: build
	./bin/$(BINARY) check --config examples/adeoluo.yaml --readme examples/adeoluo.README.md \
		--diagrams-dir assets --mermaid-config examples/mermaid.config.json

# Cross-compile release archives and a sha256 checksum file.
# Usage: make release VERSION=v0.1.0
release:
	@case "$(VERSION)" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "VERSION must look like v1.2.3 (got $(VERSION))"; exit 1;; esac
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		echo "building $$name"; \
		mkdir -p $(DIST)/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(DIST)/$$name/$(BINARY) ./cmd/$(BINARY) || exit 1; \
		cp LICENSE README.md $(DIST)/$$name/; \
		COPYFILE_DISABLE=1 tar -C $(DIST) -czf $(DIST)/$$name.tar.gz $$name || exit 1; \
		rm -rf $(DIST)/$$name; \
	done
	cd $(DIST) && shasum -a 256 *.tar.gz > checksums.txt
	@cat $(DIST)/checksums.txt

verify-release:
	cd $(DIST) && shasum -a 256 -c checksums.txt

clean:
	rm -rf bin $(DIST) coverage.out

changelog: ## Regenerate CHANGELOG.md from Conventional Commits (git-cliff)
	git cliff --output CHANGELOG.md && perl -0pi -e 's/\n+\z/\n/' CHANGELOG.md

cut-release: ## Cut a release locally: version bump, changelog, tag, GitHub release (RELEASE_VERSION=vX.Y.Z optional)
	scripts/release.sh $(RELEASE_VERSION)

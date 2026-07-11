TEST?=$$(go list ./...)
GOFMT_FILES?=$$(find . -name '*.go' | grep -vE './_local')
GO_CMD ?= go
APP_NAME = scrapent
BUILD_DIR = $(PWD)/build
SHELL := /bin/bash

all: clean tidy fmt lint security test build

setup:
	@command -v golangci-lint 2>&1 > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@command -v gosec 2>&1 > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	@command -v goreleaser 2>&1 > /dev/null || go install github.com/goreleaser/goreleaser@latest

fmt:
	$(GO_CMD)fmt -w $(GOFMT_FILES)

tidy:
	go mod tidy

lint:
	golangci-lint run --timeout 5m

lint-fix:
	golangci-lint run --fix

clean:
	rm -rf ./build cover.out

security:
	gosec -exclude G115 -exclude-dir _local -quiet ./...

test:
	go test -v -timeout 30s -coverprofile=cover.out -cover $(TEST)
	go tool cover -func=cover.out

build:
	goreleaser build --snapshot --clean

build-test:
	echo "Standard binary"
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux $(GO_CMD) build -ldflags="-s -w" -o build/scrapent_linux_amd64/scrapent ./cmd/scrapent
	du -hs build/scrapent_linux_amd64/scrapent

release:
	goreleaser release --skip=announce,publish,validate --clean

docs:
	@echo "Building documentation with MkDocs..."
	@command -v mkdocs >/dev/null 2>&1 || (echo "Error: mkdocs not found. Install with: pip install mkdocs-material mkdocs-git-revision-date-localized-plugin" && exit 1)
	@mkdocs build
	@echo "Documentation built in site/"
	@echo "To serve locally, run: make docs-serve"

docs-serve:
	@echo "Serving documentation at http://localhost:8000"
	@command -v mkdocs >/dev/null 2>&1 || (echo "Error: mkdocs not found. Install with: pip install mkdocs-material mkdocs-git-revision-date-localized-plugin" && exit 1)
	@mkdocs serve

docs-deploy:
	@echo "Deploying documentation to GitHub Pages..."
	@command -v mkdocs >/dev/null 2>&1 || (echo "Error: mkdocs not found. Install with: pip install mkdocs-material mkdocs-git-revision-date-localized-plugin" && exit 1)
	@mkdocs gh-deploy --force

docker-build:
	docker build -t scrapent:latest .

docker-run:
	docker run --rm -v $(PWD)/config.yaml:/etc/scrapent/config.yaml scrapent:latest

.PHONY: clean test security run fmt tidy lint lint-fix build build-test release docs docs-serve docs-deploy docker-build docker-run

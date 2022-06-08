SOURCE_FOLDERS?=./...
TEST_PATTERN?=.
TEST_OPTIONS?=-race -cover

.EXPORT_ALL_VARIABLES:
GOPRIVATE = github.com/Appboy
GOBIN = $(HOME)/go/bin
GO111MODULE = on

export PATH := $(GOBIN):$(PATH)

.PHONY: setup
setup: ## Install all the build and lint dependencies
	go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
	go get -u github.com/golang/dep/cmd/dep
	go get -u golang.org/x/tools/cmd/cover
	go get -u golang.org/x/tools/cmd/goimports
	go get -u github.com/AlekSi/gocoverutil

.PHONY: test
test: ## Run all the tests
	go test $(SOURCE_FOLDERS) $(TEST_OPTIONS) -timeout=1m -run $(TEST_PATTERN)
	# Run without the options too because of the explicitly non-race tests in z_*
	go test $(SOURCE_FOLDERS) -timeout=1m -run $(TEST_PATTERN)

.PHONY: cover
cover: ## Run all the tests and opens the detailed coverage report
	$(GOBIN)/gocoverutil -coverprofile=coverage.txt test -race -covermode=atomic -timeout=1m $(SOURCE_FOLDERS)
	go tool cover -html=coverage.txt

.PHONY: fmt
fmt: ## gofmt and goimports all go files
	find . -name '*.go' -not -wholename './vendor/*' | while read -r file; do gofmt -w -s "$$file"; goimports -w "$$file"; done

.PHONY: lint
lint: ## Run all the linters
	$(GOBIN)/golangci-lint run --deadline=15m --disable=errcheck

.PHONY: ci
ci: test lint ## Run all the tests and code checks

.PHONY: install
install: ## Install to $GOPATH/src
	go install ./...

.PHONY: package
package: ## Package the executable to PACKAGE_DIR
	go clean
	CGO_ENABLED=0 GOOS=linux go build -ldflags "-s -w" -a -x -installsuffix cgo -o $(PACKAGE_DIR)

# Absolutely awesome: http://marmelab.com/blog/2016/02/29/auto-documented-makefile.html
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

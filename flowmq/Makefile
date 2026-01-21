.PHONY: deps clean docs fmt lint

GOMAXPROCS         ?= 1
WEBSITE_DIR        ?= ./website
DEST_DIR           ?= ./target
PATHINSTBIN        = $(DEST_DIR)/bin
PATHINSTTOOLS      = $(DEST_DIR)/tools
PATHINSTDOCKER     = $(DEST_DIR)/docker

VERSION   := $(shell git describe --tags || echo "v0.0.0")

VER_FLAGS = -X main.Version=$(VERSION)

LD_FLAGS   ?= -w -s
GO_FLAGS   ?=
DOCS_FLAGS ?=

APPS = bento

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Go mod tidy
	@go mod tidy

SOURCE_FILES = $(shell find . -name "*.go" -type f 2>/dev/null)
TEMPLATE_FILES = $(shell find internal/impl -type f -name "template_*.yaml")

$(PATHINSTBIN)/%: $(SOURCE_FILES)
	@go build $(GO_FLAGS) -ldflags "$(LD_FLAGS) $(VER_FLAGS)" -o $@ ./main.go

$(APPS): %: $(PATHINSTBIN)/%

TOOLS = bento_docs_gen
tools: $(TOOLS)

$(PATHINSTTOOLS)/%: $(SOURCE_FILES)
	@go build $(GO_FLAGS) -ldflags "$(LD_FLAGS) $(VER_FLAGS)" -o $@ ./cmd/tools/$*

$(TOOLS): %: $(PATHINSTTOOLS)/%

fmt:
	@go list -f {{.Dir}} ./... | xargs -I{} gofmt -w -s {}
	@go list -f {{.Dir}} ./... | xargs -I{} goimports -w -local github.com/warpstreamlabs/bento {}
	@go mod tidy

lint: ## Run Go linter
	@go vet $(GO_FLAGS) ./...
	@golangci-lint -j $(GOMAXPROCS) run --fix -c .golangci.yml ./...

clean:
	rm -rf $(PATHINSTBIN)
	rm -rf $(DEST_DIR)/dist
	rm -rf $(DEST_DIR)/tools
	rm -rf $(DEST_DIR)/serverless
	rm -rf $(WEBSITE_DIR)/static/playground

docs: $(APPS) $(TOOLS)
	@$(PATHINSTTOOLS)/bento_docs_gen $(DOCS_FLAGS)
	@$(PATHINSTBIN)/bento lint --deprecated "./config/examples/*.yaml" \
		"$(WEBSITE_DIR)/cookbooks/**/*.md" \
		"$(WEBSITE_DIR)/docs/**/*.md"
	@$(PATHINSTBIN)/bento template lint "./config/template_examples/*.yaml"

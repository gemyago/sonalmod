.PHONY: lint test

include ../../../build/make/golangci-lint.mk

cover_dir=.cover
cover_profile=$(cover_dir)/profile.out
cover_html=$(cover_dir)/coverage.html

# Tests should not be taking that long
DEFAULT_TESTS_TIMEOUT ?= 10s

.DEFAULT_GOAL := all

all: test

# CI (e.g. golangci-lint-action install-only) provides golangci-lint on PATH; locally use the repo-pinned root bin/.
ifeq ($(CI),true)
lint:
	golangci-lint run
else
lint: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run
endif

$(cover_dir):
	mkdir -p $(cover_dir)

go-test-coverage=go run github.com/vladopajic/go-test-coverage/v2

test: $(cover_dir)
	TZ=US/Alaska go test -timeout=${DEFAULT_TESTS_TIMEOUT} -shuffle=on -failfast -coverpkg=./... -coverprofile=$(cover_profile) -covermode=atomic ./...
	$(go-test-coverage) --config .testcoverage.yaml --profile $(cover_profile)

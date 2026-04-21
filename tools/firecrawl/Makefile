.PHONY: test

cover_dir=.cover
cover_profile=$(cover_dir)/profile.out
cover_html=$(cover_dir)/coverage.html

# Tests should not be taking that long
DEFAULT_TESTS_TIMEOUT ?= 10s

.DEFAULT_GOAL := all

all: test

# Installing from sources is not recommended, binary is a preferred approach.
# More info is here https://golangci-lint.run/docs/welcome/install/#install-from-sources
bin/golangci-lint: .golangci-version
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s $(shell cat .golangci-version)

# CI (e.g. golangci-lint-action install-only) provides golangci-lint on PATH; locally use pinned bin/.
ifeq ($(CI),true)
lint:
	golangci-lint run
else
lint: bin/golangci-lint
	bin/golangci-lint run
endif

$(cover_dir):
	mkdir -p $(cover_dir)

go-test-coverage=go run github.com/vladopajic/go-test-coverage/v2

test: $(cover_dir)
	TZ=US/Alaska go test -timeout=${DEFAULT_TESTS_TIMEOUT} -shuffle=on -failfast -coverpkg=./internal/... -coverprofile=$(cover_profile) -covermode=atomic ./...
	$(go-test-coverage) --config .testcoverage.yaml --profile $(cover_profile)

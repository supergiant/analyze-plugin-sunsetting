SHELL := /bin/sh

MAKEFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))
CURRENT_DIR := $(patsubst %/,%,$(dir $(MAKEFILE_PATH)))

DOCKER_IMAGE_NAME := $(if ${TRAVIS_REPO_SLUG},${TRAVIS_REPO_SLUG},supergiant/analyze-plugin-sunsetting)
DOCKER_IMAGE_TAG := $(if ${TAG},${TAG},$(shell git describe --tags --always | tr -d v || echo 'latest'))

GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")

COVERALLS_TOKEN := $(if ${COVERALLS_TOKEN},${COVERALLS_TOKEN},empty)

define LINT
	@echo "Running code linters..."
	golangci-lint run
endef


define GOIMPORTS
	goimports -v -w -local github.com/supergiant/analyze-plugin-sunsetting -l $(GO_FILES)
endef

define TOOLS
		if [ ! -x "`which golangci-lint 2>/dev/null`" ]; \
        then \
        	echo "golangci-lint linter not found."; \
        	echo "Installing linter... into ${GOPATH}/bin"; \
        	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b ${GOPATH}/bin  v1.16.0 ; \
        fi

        if [ ! -x "`which goveralls 2>/dev/null`" ]; \
        then \
        	echo "goveralls not found."; \
        	echo "Installing goveralls... into ${GOPATH}/bin"; \
        	GO111MODULE=off go get -u github.com/mattn/goveralls ; \
        fi
endef


.PHONY: default
default: lint


.PHONY: lint
lint: tools
	@$(call LINT)


.PHONY: test-cover
test-cover:
	go test -covermode=count -coverprofile=coverage.out -mod=vendor -tags=dev ./...
ifneq ($(COVERALLS_TOKEN),empty)
	goveralls -coverprofile=coverage.out -service=travis-ci -repotoken $(COVERALLS_TOKEN)
endif

.PHONY: test
test:
	go test -mod=vendor -count=1 -tags=dev -race ./...

.PHONY: tools
tools:
	@$(call TOOLS)

.PHONY: goimports
goimports:
	@$(call GOIMPORTS)

.PHONY: build-image
build-image: build-ui gen-assets build
	docker build -t $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) -f ./Dockerfile .
	docker tag $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG) $(DOCKER_IMAGE_NAME):latest

.PHONY: build
build:
	./scripts/build.sh

.PHONY: gen-assets
gen-assets:
	./scripts/gen-assets.sh

.PHONY: docker-push
docker-push:
	./scripts/docker_push.sh

.PHONY: build-ui
build-ui:
	./scripts/build-ui.sh

.PHONY: push
push:
	docker push $(DOCKER_IMAGE_NAME):$(DOCKER_IMAGE_TAG)

.PHONY: gofmt
gofmt:
	go fmt ./...

.PHONY: fmt
fmt: gofmt goimports

.PHONY: push-release
push-release:
	./scripts/push_release.sh

.PHONY: test-windows
test-windows:
	docker run --rm -it --name analyze_sunsetting_test \
    		--mount type=bind,src=${CURRENT_DIR},dst=/go/src/github.com/supergiant/analyze-plugin-sunsetting/ \
    		--env GO111MODULE=on \
    		--workdir /go/src/github.com/supergiant/analyze-plugin-sunsetting/ \
    		golang:1.11.8 \
    		sh -c "go test -mod=vendor -count=1 -tags=dev -race ./..."

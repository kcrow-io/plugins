#!/usr/bin/make -f

# Copyright 2022 Authors of kcrow
# SPDX-License-Identifier: Apache-2.0


include Makefile.defs

all: build

.PHONY: all build 

START_NUM ?= 06
BIN_SUBDIRS := cmd/memory cmd/limit cmd/escape 


build: fmt tidy
	@counter=$(START_NUM); \
	for DIR in $(BIN_SUBDIRS); do \
		DIR_NAME=$${DIR##*/}; \
		for PLATFORM in $(BUILD_PLATFORMS); do \
			mkdir -p $(ROOT_DIR)/bin/$${PLATFORM}; \
			NUM=$$(printf "%02d" $$counter); \
			BINARY_NAME=$${NUM}-$${DIR_NAME}; \
			cp -f $(ROOT_DIR)/$$DIR/*.conf $(ROOT_DIR)/bin/$${PLATFORM}/ 2>/dev/null || true; \
			echo "Building \"$${BINARY_NAME}\" for $${PLATFORM}"; \
			GOOS=$${PLATFORM%/*} GOARCH=$${PLATFORM#*/} \
			$(GO_BUILD) -o $(ROOT_DIR)/bin/$${PLATFORM}/$${BINARY_NAME} $(ROOT_DIR)/$$DIR; \
		done; \
		counter=$$((counter+1)); \
	done
	@echo "Build complete."

out:
	@mkdir -p out

download: ## Downloads the dependencies
	@go mod download

tidy: ## Cleans up go.mod and go.sum
	@go mod tidy

fmt: ## Formats all code with go fmt
	@go fmt ./...

# ============ build-image ============
.PHONY: image
image:
	$(CONTAINER_ENGINE) build --platform $(IMAGE_PLATFORMS) \
			--file $(ROOT_DIR)/Dockerfile \
			--push \
			--tag $(IMAGE):$(IMAGE_TAG) $(ROOT_DIR)

#============ lints ====================
lint: fmt tidy download ## Lints all code with golangci-lint
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint run

govulncheck: ## Vulnerability detection using govulncheck
	@go run golang.org/x/vuln/cmd/govulncheck ./...

test: ## Runs all tests
	@go test $(ARGS) ./...

ci: lint test govulncheck ## Executes vulnerability scan, lint, test and generates reports

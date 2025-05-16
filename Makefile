#!/usr/bin/make -f

# Copyright 2022 Authors of kcrow
# SPDX-License-Identifier: Apache-2.0


include Makefile.defs

all: build

.PHONY: all build 

CONTROLLER_BIN_SUBDIRS := cmd/override cmd/escape

SUBDIRS := $(CONTROLLER_BIN_SUBDIRS)

build: vendor
	@for DIR in $(SUBDIRS); do \
		for PLATFORM in $(BUILD_PLATFORMS); do \
			mkdir -p $(ROOT_DIR)/bin/$${PLATFORM}; \
			echo "Building \"$${DIR##*/}\" for $${PLATFORM}"; \
			GOOS=$${PLATFORM%/*} GOARCH=$${PLATFORM#*/} \
			$(GO_BUILD) -o $(ROOT_DIR)/bin/$${PLATFORM} $(ROOT_DIR)/$$DIR; \
		done; \
	done
	@echo "Build complete."

vendor:
	@$(GO) mod vendor

# ============ build-image ============
.PHONY: image
image:
	@echo "Build Image ${IMAGE##*/} with commit $(GIT_COMMIT_VERSION)"
	$(CONTAINER_ENGINE) build --platform $(IMAGE_PLATFORMS) \
			--build-arg GIT_COMMIT_VERSION=$(GIT_COMMIT_VERSION) \
			--build-arg GIT_COMMIT_TIME=$(GIT_COMMIT_TIME) \
			--build-arg VERSION=$(GIT_COMMIT_VERSION) \
			--file $(ROOT_DIR)/Dockerfile \
			--tag $(IMAGE):$(IMAGE_TAG) $(ROOT_DIR) ; \
	@echo "Image $(IMAGE):$(IMAGE_TAG) build success" 


#============ lints ====================
.PHONY: lint-go
lint-go:
	$(QUIET) $(GO_VET) \
    ./cmd/... \
    ./pkg/... \
    ./plugins/...
	@$(ECHO_CHECK) golangci-lint
	$(QUIET) golangci-lint run

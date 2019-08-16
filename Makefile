# Copyright 2016 The OPA Authors.  All rights reserved.
# Use of this source code is governed by an Apache2
# license that can be found in the LICENSE file.

VERSION := 0.14.0-dev

GO := go
GOVERSION := 1.12.9
GOARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)

DOCKER_INSTALLED := $(shell hash docker 2>/dev/null && echo 1 || echo 0)
DOCKER := docker

BIN := opa_$(GOOS)_$(GOARCH)

REPOSITORY := openpolicyagent
IMAGE := $(REPOSITORY)/opa

BUILD_COMMIT := $(shell ./build/get-build-commit.sh)
BUILD_TIMESTAMP := $(shell ./build/get-build-timestamp.sh)
BUILD_HOSTNAME := $(shell ./build/get-build-hostname.sh)

RELEASE_BUILD_IMAGE := golang:$(GOVERSION)

LDFLAGS := "-X github.com/open-policy-agent/opa/version.Version=$(VERSION) \
	-X github.com/open-policy-agent/opa/version.Vcs=$(BUILD_COMMIT) \
	-X github.com/open-policy-agent/opa/version.Timestamp=$(BUILD_TIMESTAMP) \
	-X github.com/open-policy-agent/opa/version.Hostname=$(BUILD_HOSTNAME)"

GO15VENDOREXPERIMENT := 1
export GO15VENDOREXPERIMENT

######################################################
#
# Development targets
#
######################################################

# If you update the 'all' target check/update the call in Dockerfile.build target
# to make sure they're consistent.
.PHONY: all
all: deps build test perf check

.PHONY: version
version:
	@echo $(VERSION)

.PHONY: deps
deps:
	@./build/install-deps.sh

.PHONY: wasm-build
wasm-build:
ifeq ($(DOCKER_INSTALLED), 1)
	@$(MAKE) -C wasm build
	cp wasm/_obj/opa.wasm internal/compiler/wasm/opa/opa.wasm
else
	@echo "Docker not installed. Skipping OPA-WASM library build."
endif

.PHONY: generate
generate: wasm-build
	$(GO) generate

.PHONY: build
build: go-build

.PHONY: go-build
go-build: generate
	$(GO) build -o $(BIN) -ldflags $(LDFLAGS)

.PHONY: image
image: build-linux
	@$(MAKE) image-quick

.PHONY: install
install: generate
	$(GO) install -ldflags $(LDFLAGS)

.PHONY: test
test: opa-wasm-test go-test wasm-test

.PHONE: fuzzit-local-regression
fuzzit-local-regression:
	./build/fuzzit.sh local-regression

.PHONE: fuzzit-fuzzing
fuzzit-fuzzing:
	./build/fuzzit.sh fuzzing

.PHONY: opa-wasm-test
opa-wasm-test:
ifeq ($(DOCKER_INSTALLED), 1)
	@$(MAKE)  -C wasm test
else
	@echo "Docker not installed. Skipping OPA-WASM library test."
endif

.PHONY: go-test
go-test: generate
	$(GO) test ./...

.PHONY: wasm-test
wasm-test: generate
ifeq ($(DOCKER_INSTALLED), 1)
	@./build/run-wasm-tests.sh
else
	@echo "Docker not installed. Skipping WASM-based test execution."
endif

.PHONY: perf
perf: generate
	$(GO) test -run=- -bench=. -benchmem ./...

.PHONY: check
check: check-fmt check-vet check-lint

.PHONY: check-fmt
check-fmt:
	./build/check-fmt.sh

.PHONY: check-vet
check-vet:
	./build/check-vet.sh

.PHONY: check-lint
check-lint:
	./build/check-lint.sh

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: wasm-clean
wasm-clean:
	@$(MAKE) -C wasm clean

.PHONY: clean
clean: wasm-clean
	rm -f opa_*_*
	rm -fr _test

# The docs-% pattern target will shim to the
# makefile in ./docs
.PHONY: docs-%
docs-%:
	$(MAKE) -C docs $*

######################################################
#
# CI targets
#
######################################################

.PHONY: travis-build
travis-build: wasm-build opa-wasm-test
	# this image is used in `Dockerfile` for image-quick
	docker build -t build-$(BUILD_COMMIT) --build-arg GOVERSION=$(GOVERSION) -f Dockerfile.build .
	# the '/.' means "don't create the directory, copy its content only"
	# note: we don't bother cleaning up the container
	docker cp "$$(docker create build-$(BUILD_COMMIT)):/out/." .

.PHONY: travis-all
travis-all: travis-build wasm-test fuzzit-local-regression

.PHONY: build-linux
build-linux:
	@$(MAKE) build GOOS=linux

.PHONY: build-darwin
build-darwin:
	@$(MAKE) build GOOS=darwin

.PHONY: build-windows
build-windows:
	@$(MAKE) build GOOS=windows
	mv opa_windows_$(GOARCH) opa_windows_$(GOARCH).exe

.PHONY: image-quick
image-quick:
	docker build --build-arg BUILD_COMMIT=$(BUILD_COMMIT) -t $(IMAGE):$(VERSION) .
	docker build --build-arg BUILD_COMMIT=$(BUILD_COMMIT) -t $(IMAGE):$(VERSION)-debug --build-arg VARIANT=:debug .
	docker build --build-arg BUILD_COMMIT=$(BUILD_COMMIT) -t $(IMAGE):$(VERSION)-rootless --build-arg USER=1 .

.PHONY: push
push:
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):$(VERSION)-debug
	docker push $(IMAGE):$(VERSION)-rootless

.PHONY: tag-latest
tag-latest:
	docker tag $(IMAGE):$(VERSION) $(IMAGE):latest
	docker tag $(IMAGE):$(VERSION)-debug $(IMAGE):latest-debug
	docker tag $(IMAGE):$(VERSION)-rootless $(IMAGE):latest-rootless

.PHONY: push-latest
push-latest:
	docker push $(IMAGE):latest
	docker push $(IMAGE):latest-debug
	docker push $(IMAGE):latest-rootless

.PHONY: push-binary-edge
push-binary-edge:
	aws s3 cp opa_darwin_$(GOARCH) s3://opa-releases/edge/opa_darwin_$(GOARCH)
	aws s3 cp opa_windows_$(GOARCH).exe s3://opa-releases/edge/opa_windows_$(GOARCH).exe
	aws s3 cp opa_linux_$(GOARCH) s3://opa-releases/edge/opa_linux_$(GOARCH)

.PHONY: tag-edge
tag-edge:
	docker tag $(IMAGE):$(VERSION) $(IMAGE):edge
	docker tag $(IMAGE):$(VERSION)-debug $(IMAGE):edge-debug
	docker tag $(IMAGE):$(VERSION)-rootless $(IMAGE):edge-rootless

.PHONY: push-edge
push-edge:
	docker push $(IMAGE):edge
	docker push $(IMAGE):edge-debug
	docker push $(IMAGE):edge-rootless

.PHONY: docker-login
docker-login:
	@docker login -u ${DOCKER_USER} -p ${DOCKER_PASSWORD}

.PHONY: push-image
push-image: docker-login image-quick push

.PHONY: deploy-travis
deploy-travis: push-image tag-edge push-edge push-binary-edge

.PHONY: release-travis
release-travis: push-image tag-latest push-latest

.PHONY: release-bugfix-travis
release-bugfix-travis: deploy-travis

######################################################
#
# Release targets
#
######################################################

.PHONY: release
release:
	docker run -it --rm \
		-v $(PWD)/_release/$(VERSION):/_release/$(VERSION) \
		-v $(PWD):/_src \
		$(RELEASE_BUILD_IMAGE) \
		/_src/build/build-release.sh --version=$(VERSION) --output-dir=/_release/$(VERSION) --source-url=/_src

.PHONY: release-local
release-local:
	docker run -it --rm \
		-v $(PWD)/_release/$(VERSION):/_release/$(VERSION) \
		-v $(PWD):/_src \
		$(RELEASE_BUILD_IMAGE) \
		/_src/build/build-release.sh --output-dir=/_release/$(VERSION) --source-url=/_src

.PHONY: release-patch
release-patch:
	@docker run -it --rm \
		-e LAST_VERSION=$(LAST_VERSION) \
		-v $(PWD):/_src \
		python:2.7 \
		/_src/build/gen-release-patch.sh --version=$(VERSION) --source-url=/_src

.PHONY: dev-patch
dev-patch:
	@docker run -it --rm \
		-v $(PWD):/_src \
		python:2.7 \
		/_src/build/gen-dev-patch.sh --version=$(VERSION) --source-url=/_src

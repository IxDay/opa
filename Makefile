# Copyright 2015 The OPA Authors.  All rights reserved.
# Use of this source code is governed by an Apache2
# license that can be found in the LICENSE file.

PACKAGES := github.com/open-policy-agent/opa/jsonlog/.../ \
	github.com/open-policy-agent/opa/cmd/.../

BUILD_COMMIT := $(shell ./build/get-build-commit.sh)
BUILD_TIMESTAMP := $(shell ./build/get-build-timestamp.sh)
BUILD_HOSTNAME := $(shell ./build/get-build-hostname.sh)

LDFLAGS := -ldflags "-X github.com/open-policy-agent/opa/version.Vcs=$(BUILD_COMMIT) \
	-X github.com/open-policy-agent/opa/version.Timestamp=$(BUILD_TIMESTAMP) \
	-X github.com/open-policy-agent/opa/version.Hostname=$(BUILD_HOSTNAME)"

GO := go

GO15VENDOREXPERIMENT := 1
export GO15VENDOREXPERIMENT

.PHONY: all generate build test clean

all: build test

generate:
	$(GO) generate

build:
	$(GO) build -o opa $(LDFLAGS)

test:
	$(GO) test -v $(PACKAGES)

clean:
	rm -f ./opa

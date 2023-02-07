GIT_COMMIT=$(shell git rev-parse --short HEAD)

GOTEST=go test
GOCOVER=go tool cover

ARCHES=amd64 arm64
PLATFORMS=darwin linux windows

BUILDARCH?=$(shell uname -m)
BUILDPLATFORM?=$(shell uname -s)

ifeq ($(BUILDARCH),aarch64)
  BUILDARCH=arm64
endif
ifeq ($(BUILDARCH),x86_64)
  BUILDARCH=amd64
endif
ifeq ($(BUILDPLATFORM),Darwin)
  BUILDPLATFORM=darwin
endif
ifeq ($(BUILDPLATFORM),Linux)
  BUILDPLATFORM=linux
endif
ifeq ($(BUILDPLATFORM),Win)
  BUILDPLATFORM=windows
endif

ARCH ?= $(BUILDARCH)
PLATFORM ?= $(BUILDPLATFORM)

ifeq ($(ARCH),aarch64)
  override ARCH=arm64
endif
ifeq ($(ARCH),x86_64)
  override ARCH=amd64
endif
ifeq ($(PLATFORM),Darwin)
  override PLATFORM=darwin
endif
ifeq ($(PLATFORM),Linux)
  override PLATFORM=linux
endif
ifeq ($(PLATFORM),Win)
  override PLATFORM=windows
endif

VERSION ?= "0.0.1"
DEFAULTIMAGE ?= funkymcb/work-timer:$(VERSION)

.PHONY: all

all: clean cover build

test:
	$(GOTEST) -v -coverprofile=out/coverage.out ./...

cover: test
	$(GOCOVER) -func=out/coverage.out
	$(GOCOVER) -html=out/coverage.out

build:
	CGO_ENABLED=0 GOOS=$(PLATFORM) GOARCH=$(ARCH) \
	  go build -o out/work \
		-ldflags "-X 'github.com/funkymcb/work-timer/cmd.Version=${VERSION}' -X 'github.com/funkymcb/work-timer/cmd.GitCommit=${GIT_COMMIT}'" \
		-a -installsuffix cgo main.go

clean:
	rm -f ./coverage.out ./out/*
	# docker rmi $(DEFAULTIMAGE)

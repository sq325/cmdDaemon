.PHONY: arm linux vendor-build linux-arm build darwin windows run help

buildTime ?= $(shell date '+%Y-%m-%d_%H:%M:%S')
modName := $(shell go list -m)
projectName := $(shell basename $(modName))
buildGoVersion := $(shell go version|awk '{print $$3}')
SHELL_ESCAPE_SINGLE_QUOTES = sed "s/'//g"
# Escape potentially problematic values
author := $(shell git config user.name | $(SHELL_ESCAPE_SINGLE_QUOTES))
tag := $(shell git describe --tags --abbrev=0 2>/dev/null | $(SHELL_ESCAPE_SINGLE_QUOTES))
# Use the raw tag for git log, then escape the resulting commitInfo
commitInfo := $(shell git log -1 --format="%s" "$(tag)" 2>/dev/null | $(SHELL_ESCAPE_SINGLE_QUOTES))

LDFLAGS := -X 'main.projectName=$(projectName)' \
           -X 'main.buildTime=$(buildTime)' \
           -X 'main.buildGoVersion=$(buildGoVersion)' \
           -X 'main.author=$(author)' \
           -X 'main._version=$(tag)' \
           -X 'main._versionInfo=$(commitInfo)'
version ?= $(tag) # Use the escaped tag for consistency if version is also injected or used in similar contexts

build:
	@echo "--- Building ---"
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(projectName)

darwin:
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(projectName)

linux:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(projectName)

vendor-build:
	go mod tidy && go mod vendor
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -ldflags "$(LDFLAGS)" -o $(projectName)
	tar -zcvf $(projectName)_$(tag).tar.gz --exclude=./vendor/ --exclude=./.git/ ./

linux-arm:
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(projectName)

windows:
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(projectName).exe

run:
	@go run main.go

help:
	@echo "usage: make <option>"
	@echo "    help   : Show help"
	@echo "    build  : Build the binary of this project for current platform"
	@echo "    run	  : run the  project"
	@echo "    linux  : Build the amd64 linux binary of this project"
	@echo "    darwin : Build the arm64 darwin binary of this project"
	@echo "    windows : Build the arm64 windows binary of this project"
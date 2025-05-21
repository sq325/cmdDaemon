.PYONY: arm linux vendor-build linux-arm build darwin windows run help

buildTime ?= $(shell date '+%Y-%m-%d_%H:%M:%S')
modName := $(shell go list -m)
projectName := $(shell basename $(modName))
# git remote get-url origin | xargs basename -s .git
buildGoVersion := $(shell go version|awk '{print $$3}')
author := $(shell git config user.name)
tag := $(shell git describe --tags --abbrev=0 2>/dev/null)
commitInfo := $(shell git log -1 --format=%s $(tag) 2>/dev/null)
LDFLAGS := -X '$(modName)/cmd.projectName=$(projectName)' -X '$(modName)/cmd.buildTime=$(buildTime)' -X '$(modName)/cmd.buildGoVersion=$(buildGoVersion)' -X '$(modName)/cmd.author=$(author)' -X '$(modName)/cmd._version=$(tag)' -X '$(modName)/cmd._versionInfo=$(commitInfo)'
version ?= $(shell git describe --tags --abbrev=0)

build: 
	@CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(projectName)

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
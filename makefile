.PYONY: arm linux build darwin windows run help

buildTime ?= $(shell date '+%Y-%m-%d_%H:%M:%S')
buildGoVersion := $(shell go version|awk '{print $$3}')
author := $(shell git config user.name)
LDFLAGS := -X 'main.buildTime=$(buildTime)' -X 'main.buildGoVersion=$(buildGoVersion)' -X 'main.author=$(author)'


build: 
	@CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o cmdDaemon 

darwin:
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o cmdDaemon 

linux:
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o cmdDaemon 

windows:
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o cmdDaemon.exe 

run:
	@go run ./main.go

help:
	@echo "usage: make <option>"
	@echo "    help   : Show help"
	@echo "    build  : Build the binary of this project for current platform"
	@echo "    run	  : run the  project"
	@echo "    linux  : Build the amd64 linux binary of this project"
	@echo "    darwin : Build the arm64 darwin binary of this project"
BINARY_PATH := bin/scs

SOURCES := $(shell find . -name '*.go')

build: $(SOURCES)
	mkdir -p bin
	go build -o $(BINARY_PATH) cmd/main.go

config: # watch out, this might delete your configs
	mkdir -p bin/config
	cp config_sample/* bin/config

run: build
	./bin/scs

.PHONY: all
all: build config

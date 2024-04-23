BINARY_PATH := bin/scs

SOURCES := $(shell find . -name '*.go')
SAMPLE_CONFIGS := $(shell find config -name '*.toml')

run: build subdirs
	./bin/scs

build: $(SOURCES)
	mkdir -p bin
	go build -o $(BINARY_PATH) cmd/main.go

.PHONY: subdirs
subdirs:
	mkdir -p bin/log/room

.PHONY: all # watch out, this might delete your configs
all: build subdirs

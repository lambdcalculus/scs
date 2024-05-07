SERVER_BINARY := bin/scs
SERVERCTL_BINARY := bin/serverctl

SOURCES := $(shell find . -name '*.go')
CONFIGS := $(shell find . -wholename 'config_sample/*.toml')

server: $(SOURCES)
	mkdir -p bin
	go build -tags "libsqlite3" -o $(SERVER_BINARY) ./cmd/scs

server-static: $(SOURCES)
	mkdir -p bin
	go build -o $(SERVER_BINARY) ./cmd/scs

serverctl: cmd/serverctl/main.go 
	mkdir -p bin
	go build -o $(SERVERCTL_BINARY) ./cmd/serverctl

config: $(CONFIGS) # watch out, this might delete your configs
	mkdir -p bin/config
	cp config_sample/* bin/config

run: server
	./bin/scs

.PHONY: all
all: server serverctl

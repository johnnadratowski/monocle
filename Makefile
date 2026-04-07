.PHONY: build run test vet lint sync-skills skills-tarball

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/monocle ./cmd/monocle

run: build
	./bin/monocle

install:
	go install ./cmd/monocle

test:
	go test ./internal/...

vet:
	go vet ./...

lint: vet
	go build ./...

SKILL_NAMES := $(notdir $(patsubst %/SKILL.md,%,$(wildcard skills/*/SKILL.md)))
PLUGIN_AGENTS := claude codex gemini

sync-skills:
	@for agent in $(PLUGIN_AGENTS); do \
		rm -rf plugins/$$agent/skills; \
		mkdir -p plugins/$$agent/skills; \
		for skill in $(SKILL_NAMES); do \
			cp -r skills/$$skill plugins/$$agent/skills/$$skill; \
		done; \
	done

skills-tarball:
	mkdir -p dist
	tar -czf dist/skills.tar.gz --exclude='*.go' -C skills .

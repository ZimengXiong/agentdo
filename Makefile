GO ?= go
BIN := agentdo
VERSION := $(shell $(GO) run ./cmd/agentdo version)
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DIST_DIR := dist
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64

.PHONY: build test install package clean

build:
	$(GO) build -o $(BIN) ./cmd/agentdo

test:
	$(GO) test ./...

install: build
	install -d $(BINDIR)
	install -m 0755 $(BIN) $(BINDIR)/$(BIN)

package:
	rm -rf $(DIST_DIR)
	mkdir -p $(DIST_DIR)
	@set -e; \
	for target in $(TARGETS); do \
		goos=$${target%/*}; \
		goarch=$${target#*/}; \
		stage="$(DIST_DIR)/$(BIN)_$(VERSION)_$${goos}_$${goarch}"; \
		mkdir -p "$$stage"; \
		GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0 $(GO) build -o "$$stage/$(BIN)" ./cmd/agentdo; \
		cp README.md "$$stage/README.md"; \
		tar -C $(DIST_DIR) -czf "$(DIST_DIR)/$(BIN)_$(VERSION)_$${goos}_$${goarch}.tar.gz" "$(BIN)_$(VERSION)_$${goos}_$${goarch}"; \
		rm -rf "$$stage"; \
	done
	cd $(DIST_DIR) && shasum -a 256 *.tar.gz > SHA256SUMS

clean:
	rm -rf $(DIST_DIR) $(BIN)

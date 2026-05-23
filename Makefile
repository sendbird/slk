.PHONY: build test lint run clean install uninstall

BINARY=slk
BUILD_DIR=bin
# Canonical local install path. Every agent / script that installs slk
# locally should target this exact path so multiple builds can never
# disagree about which binary is on $PATH.
INSTALL_PATH=$(HOME)/.local/bin/$(BINARY)

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/slk

# Build a stripped, reproducible binary and copy it to INSTALL_PATH,
# replacing whatever is there. `install -m 755` keeps the canonical
# permissions across shells.
install:
	go build -trimpath -ldflags='-s -w' -o $(BUILD_DIR)/$(BINARY) ./cmd/slk
	install -m 755 $(BUILD_DIR)/$(BINARY) $(INSTALL_PATH)
	@echo "installed: $(INSTALL_PATH)"

uninstall:
	rm -f $(INSTALL_PATH)

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

run: build
	./$(BUILD_DIR)/$(BINARY)

clean:
	rm -rf $(BUILD_DIR)

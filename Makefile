.PHONY: build install test clean help release clean-release

BINARY_NAME=l2tp-client
CMD_PATH=./cmd/l2tp-client
VERSION?=0.1.0
BUILD_TIME=$(shell date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DIST_DIR=dist
RELEASE_NAME=$(BINARY_NAME)-$(VERSION)

LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: deps ## Build the binary (uses local go-l2tp via replace in go.mod)
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(CMD_PATH)
	@echo "Build complete: $(BINARY_NAME)"

# Release: build static Linux binary and tarball. Requires local go-l2tp clone (see go.mod replace).
release: ## Build release tarball for Linux (copy to server, no Go required)
	@mkdir -p $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64
	@echo "Building static binary for linux/amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/$(BINARY_NAME) $(CMD_PATH)
	@cp example-config.toml $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/
	@echo "Install (no Go or internet required on this machine):" > $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@echo "  1. sudo cp l2tp-client /usr/local/bin/" >> $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@echo "  2. Copy example-config.toml to your config, set server/user/password." >> $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@echo "  3. Kernel L2TP: sudo modprobe l2tp_core l2tp_netlink l2tp_eth l2tp_ip l2tp_ip6" >> $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@echo "  4. Install pppd if needed: sudo apt install ppp" >> $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@echo "  5. sudo l2tp-client connect --config your.toml" >> $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64/INSTALL.txt
	@cd $(DIST_DIR) && tar -czvf $(RELEASE_NAME)-linux-amd64.tar.gz $(RELEASE_NAME)-linux-amd64
	@echo "Release: $(DIST_DIR)/$(RELEASE_NAME)-linux-amd64.tar.gz"

install: build ## Install the binary to /usr/local/bin (requires sudo)
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installation complete"

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

test-root: ## Run tests requiring root (for kernel integration tests)
	@echo "Running root tests..."
	@sudo go test -exec sudo -run TestRequiresRoot ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean: clean-release ## Clean build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@go clean
	@echo "Clean complete"

clean-release: ## Remove dist/ and release tarballs
	@rm -rf $(DIST_DIR)
	@echo "Release artifacts removed"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies downloaded"

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Formatting complete"

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete"

lint: fmt vet ## Run linter checks

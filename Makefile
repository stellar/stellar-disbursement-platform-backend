# Variables
export GO111MODULE=on
BINARY_NAME := stellar-disbursement-platform
BUILD_DIR := build
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")
GO_PACKAGES := $(shell go list ./... | grep -v /vendor/)
LABEL ?= $(shell git rev-parse --short HEAD)$(and $(shell git status -s),-dirty-$(shell id -u -n))

# Colors for output
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
CYAN   := $(shell tput -Txterm setaf 6)
RESET  := $(shell tput -Txterm sgr0)

# Ensure build directory exists
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Build targets
.PHONY: build
build: $(BUILD_DIR) ## Build the binary
	go build -o $(BUILD_DIR)/$(BINARY_NAME) -ldflags "-X main.GitCommit=$(LABEL)" .
	@echo "$(GREEN)Build successful!$(RESET)"
	@echo "Binary location: $(CYAN)$(BUILD_DIR)/$(BINARY_NAME)$(RESET)"

.PHONY: clean
clean: ## Clean build artifacts
	go clean
	rm -rf $(BUILD_DIR)
	@echo "$(GREEN)Cleanup complete!$(RESET)"

.PHONY: help
help: ## Show this help
	@echo ''
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} { \
		if (/^[a-zA-Z_-]+:.*?##.*$$/) {printf "    ${YELLOW}%-20s${GREEN}%s${RESET}\n", $$1, $$2} \
		else if (/^## .*$$/) {printf "  ${CYAN}%s${RESET}\n", substr($$1,4)} \
		}' $(MAKEFILE_LIST)

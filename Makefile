# Check if we need to prepend docker command with sudo
SUDO := $(shell docker version >/dev/null 2>&1 || echo "sudo")

# If LABEL is not provided set default value
LABEL ?= $(shell git rev-parse --short HEAD)$(and $(shell git status -s),-dirty-$(shell id -u -n))
# If TAG is not provided set default value
TAG ?= stellar/stellar-disbursement-platform:$(LABEL)
# https://github.com/opencontainers/image-spec/blob/master/annotations.md
BUILD_DATE := $(shell date -u +%FT%TZ)

LOCAL_MODULE := github.com/stellar/stellar-disbursement-platform-backend
GOPATH_BIN := $(shell go env GOPATH)/bin
export PATH := $(GOPATH_BIN):$(PATH)

# Always run these targets (they don't create files named after the target)
.PHONY: docker-build docker-push go-install setup go-install-tools \
	go-test go-lint go-shadow go-mod go-deadcode go-exhaustive go-goimports go-build go-check ci

docker-build:
	$(SUDO) docker build -f Dockerfile.development --pull --label org.opencontainers.image.created="$(BUILD_DATE)" -t $(TAG) --build-arg GIT_COMMIT=$(LABEL) .

docker-push:
	$(SUDO) docker push $(TAG)

go-install:
	go build -o $(GOPATH)/bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$(LABEL)" .

setup:
	go run tools/sdp-setup/main.go

go-install-tools:
	@echo ""
	@echo "🔧 Installing CI tools..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.2
	go install golang.org/x/tools/go/analysis/passes/shadow/cmd/shadow@v0.31.0
	go install github.com/nishanths/exhaustive/cmd/exhaustive@v0.12.0
	go install golang.org/x/tools/cmd/deadcode@v0.31.0
	go install golang.org/x/tools/cmd/goimports@v0.31.0
	go install gotest.tools/gotestsum@v1.11.0
	@echo "✅ All CI tools installed"

go-build:
	@echo ""
	@echo "🔨 Building project..."
	go build ./...
	@echo "✅ Build completed successfully"

go-test:
	@echo ""
	@echo "🧪 Running unit tests..."
	gotestsum --format-hide-empty-pkg --format pkgname-and-test-fails -- -timeout 5m ./...
	@echo "✅ Unit tests completed successfully"

go-lint:
	@echo ""
	@echo "🔍 Running golangci-lint..."
	golangci-lint run
	@echo "✅ golangci-lint completed successfully"

go-shadow:
	@echo ""
	@echo "🌑 Running shadow variable detection..."
	@output=$$(shadow ./... 2>&1 | grep -v "generated.go" || true); \
	if [ -n "$$output" ]; then \
		echo "$$output"; \
		exit 1; \
	fi
	@echo "✅ Shadow check completed successfully"

go-mod:
	@echo ""
	@echo "📦 Verifying Go modules..."
	./gomod.sh
	@echo "✅ Module verification completed successfully"

go-deadcode:
	@echo ""
	@echo "💀 Running dead code detection..."
	@output=$$(deadcode -test ./... 2>&1 | grep -v "UnmarshalUInt32" || true); \
	if [ -n "$$output" ]; then \
		echo "$$output"; \
		exit 1; \
	fi
	@echo "✅ Dead code check completed successfully"

go-exhaustive:
	@echo ""
	@echo "🔄 Running exhaustive enum checking..."
	exhaustive -default-signifies-exhaustive ./...
	@echo "✅ Exhaustive check completed successfully"

go-goimports:
	@echo ""
	@echo "📐 Checking goimports compliance..."
	@non_compliant=$$(find . -type f -name "*.go" ! -path "*mock*" | xargs goimports -local "$(LOCAL_MODULE)" -l); \
	if [ -n "$$non_compliant" ]; then \
		echo "🚨 The following files are not compliant with goimports:"; \
		echo "$$non_compliant"; \
		echo "Run 'goimports -local \"$(LOCAL_MODULE)\" -w <file>' to fix."; \
		exit 1; \
	fi
	@echo "✅ All files are compliant with goimports"

go-check: go-mod go-lint go-shadow go-exhaustive go-deadcode go-goimports
	@echo ""
	@echo "🎉🎉🎉 All Go checks completed successfully! 🎉🎉🎉"

ci: go-check go-build go-test
	@echo ""
	@echo "🎉🎉🎉 Full CI pipeline completed successfully! 🎉🎉🎉"

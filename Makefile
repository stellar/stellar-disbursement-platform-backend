# Check if we need to prepend docker command with sudo
SUDO := $(shell docker version >/dev/null 2>&1 || echo "sudo")

# If LABEL is not provided set default value
LABEL ?= $(shell git rev-parse --short HEAD)$(and $(shell git status -s),-dirty-$(shell id -u -n))
# If TAG is not provided set default value
TAG ?= stellar/stellar-disbursement-platform:$(LABEL)
# https://github.com/opencontainers/image-spec/blob/master/annotations.md
BUILD_DATE := $(shell date -u +%FT%TZ)

# Always run these targets (they don't create files named after the target)
.PHONY: docker-build docker-push go-install setup

docker-build:
	$(SUDO) docker build -f Dockerfile.development --pull --label org.opencontainers.image.created="$(BUILD_DATE)" -t $(TAG) --build-arg GIT_COMMIT=$(LABEL) .

docker-push:
	$(SUDO) docker push $(TAG)

go-install:
	go build -o $(GOPATH)/bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$(LABEL)" .

setup:
	go run tools/sdp-setup/main.go

go-test:
	@echo ""
	@echo "🧪 Running unit tests..."
	gotestsum --format-hide-empty-pkg --format pkgname-and-test-fails
	@echo "✅ Unit tests completed successfully"

go-lint:
	@echo ""
	@echo "🔍 Running golangci-lint..."
	golangci-lint run
	@echo "✅ golangci-lint completed successfully"

go-shadow:
	@echo ""
	@echo "🌑 Running shadow variable detection..."
	shadow ./...
	@echo "✅ Shadow check completed successfully"

go-mod:
	@echo ""
	@echo "📦 Verifying Go modules..."
	./gomod.sh
	@echo "✅ Module verification completed successfully"

go-deadcode:
	@echo ""
	@echo "💀 Running dead code detection..."
	deadcode -test ./...
	@echo "✅ Dead code check completed successfully"

go-exhaustive:
	@echo ""
	@echo "🔄 Running exhaustive enum checking..."
	exhaustive -default-signifies-exhaustive ./...
	@echo "✅ Exhaustive check completed successfully"

go-check: go-test go-lint go-shadow go-mod go-deadcode go-exhaustive
	@echo ""
	@echo "🎉🎉🎉 All Go checks completed successfully! 🎉🎉🎉"

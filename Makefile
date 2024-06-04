# Check if we need to prepend docker command with sudo
SUDO := $(shell docker version >/dev/null 2>&1 || echo "sudo")

# If LABEL is not provided set default value
LABEL ?= $(shell git rev-parse --short HEAD)$(and $(shell git status -s),-dirty-$(shell id -u -n))
# If TAG is not provided set default value
TAG ?= stellar/stellar-disbursement-platform:$(LABEL)
# https://github.com/opencontainers/image-spec/blob/master/annotations.md
BUILD_DATE := $(shell date -u +%FT%TZ)

docker-build:
	$(SUDO) docker build -f Dockerfile.development --pull --label org.opencontainers.image.created="$(BUILD_DATE)" -t $(TAG) --build-arg GIT_COMMIT=$(LABEL) .

docker-push:
	$(SUDO) docker push $(TAG)

go-install:
	go build -o $(GOPATH)/bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$(LABEL)" .

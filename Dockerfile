# To build:
#    make docker-build
# To push:
#    make docker-push

FROM golang:1.23.2-bullseye AS build

# Declare the build argument
ARG GIT_COMMIT

WORKDIR /src/stellar-disbursement-platform
ADD go.mod go.sum ./
RUN go mod download
ADD . ./

# Assign GIT_COMMIT in the environment, falling back to the current commit hash if empty
RUN if [ -z "$GIT_COMMIT" ]; then \
    GIT_COMMIT=$(git rev-parse --short HEAD)$( [ -n "$(git status -s)" ] && echo "-dirty-$(id -u -n)" ); \
    fi && \
    echo "Building with commit: $GIT_COMMIT" && \
    go build -o /bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=${GIT_COMMIT}" .

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates
COPY --from=build /bin/stellar-disbursement-platform /app/
EXPOSE 8001
WORKDIR /app
ENTRYPOINT ["./stellar-disbursement-platform"]

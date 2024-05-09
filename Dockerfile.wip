# Stage 1: Build the Go application
FROM golang:1.22.1-bullseye AS build
ARG GIT_COMMIT

WORKDIR /src/stellar-disbursement-platform
ADD go.mod go.sum ./
RUN go mod download
ADD . ./
RUN go build -o /bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$GIT_COMMIT" .

# Stage 2: Setup the development environment with Delve for debugging
FROM golang:1.22.1-bullseye AS development

WORKDIR /app/github.com/stellar/stellar-disbursement-platform
# Copy the built executable and all source files for debugging
COPY --from=build /src/stellar-disbursement-platform /app/github.com/stellar/stellar-disbursement-platform
# Build a debug version of the binary
RUN go build -gcflags="all=-N -l" -o stellar-disbursement-platform .
# Install Delve
RUN go install github.com/go-delve/delve/cmd/dlv@latest
# Ensure the binary has executable permissions
RUN chmod +x /app/github.com/stellar/stellar-disbursement-platform/stellar-disbursement-platform
EXPOSE 8001 2345
ENTRYPOINT ["/go/bin/dlv", "debug", "./stellar-disbursement-platform", "serve", "--headless", "--listen=:2345", "--api-version=2", "--log"]

# Stage 3: Create a production image using Ubuntu
FROM ubuntu:22.04 AS production
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates
COPY --from=build /bin/stellar-disbursement-platform /app/stellar-disbursement-platform
EXPOSE 8001
WORKDIR /app
ENTRYPOINT ["./stellar-disbursement-platform"]

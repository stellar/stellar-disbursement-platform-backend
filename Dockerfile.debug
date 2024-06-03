# Use the golang image for building and debugging
FROM golang:1.22.1-bullseye AS build
ARG GIT_COMMIT

# Set the working directory inside the Docker container
WORKDIR /app

# Copy the Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire project directory from your host machine to the Docker container
COPY . ./github.com/stellar/stellar-disbursement-platform

# Build your application
WORKDIR /app/github.com/stellar/stellar-disbursement-platform
RUN go build -gcflags="all=-N -l" -o stellar-disbursement-platform 

#install jq for automating tenant owners
RUN apt-get update && apt-get install -y jq

# Install Delve for debugging
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Ensure the binary has executable permissions
RUN chmod +x /app/github.com/stellar/stellar-disbursement-platform
RUN chmod +x /app/github.com/stellar/stellar-disbursement-platform/dev/scripts/add_test_users.sh

# Expose the ports for your application 
EXPOSE 8001

# Set the entrypoint to start Delve in headless mode for debugging
ENTRYPOINT ["/go/bin/dlv", "exec", "./stellar-disbursement-platform", "serve", "--headless", "--listen=:2345", "--api-version=2", "--log"]

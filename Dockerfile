# Stage 1: Build the Go application
FROM golang:1.22.1-bullseye AS build
ARG GIT_COMMIT

WORKDIR /src/stellar-disbursement-platform
ADD go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$GIT_COMMIT" .

# Stage 3: Create a production image using Ubuntu
FROM ubuntu:22.04 AS production
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates
COPY --from=build /bin/stellar-disbursement-platform /app/stellar-disbursement-platform
EXPOSE 8001
WORKDIR /app
ENTRYPOINT ["./stellar-disbursement-platform", "serve"]
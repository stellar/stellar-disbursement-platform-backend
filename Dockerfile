# To build:
#    make docker-build
# To push:
#    make docker-push

FROM golang:1.23.2-bullseye AS build
ARG GIT_COMMIT

WORKDIR /src/stellar-disbursement-platform
ADD go.mod go.sum ./
RUN go mod download
ADD . ./
RUN go build -o /bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$GIT_COMMIT" .


FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates
# ADD migrations/ /app/migrations/
COPY --from=build /bin/stellar-disbursement-platform /app/
EXPOSE 8001
WORKDIR /app
ENTRYPOINT ["./stellar-disbursement-platform"]

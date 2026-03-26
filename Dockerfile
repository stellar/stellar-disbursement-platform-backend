# To build:
#    make docker-build
# To push:
#    make docker-push

FROM golang:1.26-alpine AS build
ARG GIT_COMMIT

ENV CGO_ENABLED=0 GOOS=linux
WORKDIR /src/stellar-disbursement-platform
ADD go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build -o /bin/stellar-disbursement-platform -ldflags "-X main.GitCommit=$GIT_COMMIT" .


FROM alpine:3.23

RUN apk add --no-cache ca-certificates
# ADD migrations/ /app/migrations/
COPY --from=build /bin/stellar-disbursement-platform /app/
EXPOSE 8001
WORKDIR /app
ENTRYPOINT ["./stellar-disbursement-platform"]
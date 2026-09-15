FROM golang:1.25.13-trixie@sha256:9044d290beebfb6bb1d1f38d94f6c0f5a217fadff3497c73a760e5c577b24476 AS builder

WORKDIR /app
COPY go.mod go.sum ./

RUN go mod download

COPY . .

# build the package, not a file list — a file list stamps the binary as
# "command-line-arguments" with no module or vcs.revision provenance
RUN go build -trimpath -o ./tee-relay-client ./cmd/main

FROM debian:trixie@sha256:fd8f5a1df07b5195613e4b9a0b6a947d3772a151b81975db27d47f093f60c6e6 AS execution

WORKDIR /app

COPY --from=builder /app/tee-relay-client .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 10001

CMD ["./tee-relay-client"]

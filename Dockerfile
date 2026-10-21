#checkov:skip=CKV_DOCKER_2:no curl in the image, probes are orchestrator httpGet on [health]
FROM golang:1.26.10-trixie@sha256:7202554ed358a37f41c44e4910ae29bbc5ef0715f289ac637218adbb0ca132dd AS builder

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

# health probes; must match config.DefaultHealthPort
EXPOSE 8080

CMD ["./tee-relay-client"]

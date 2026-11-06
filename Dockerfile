#checkov:skip=CKV_DOCKER_2:no curl in the image, probes are orchestrator httpGet on [health]
FROM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS builder

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

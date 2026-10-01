#checkov:skip=CKV_DOCKER_2:no curl in the image, probes are orchestrator httpGet on [health]
FROM golang:1.26.8-trixie@sha256:eae2aaa6add2936cbf350dd0d2628b363461542f0c4b3c0b558957e0f2997379 AS builder

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

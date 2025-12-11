FROM golang:1.25.1-trixie@sha256:61226c61f37cb86253c4ac486ef22c47f14bfddb8f60bb4805bfc165001be758 AS builder

WORKDIR /app
COPY tee-node ./tee-node

WORKDIR /app/tee-relay-client
COPY tee-relay-client/go.mod tee-relay-client/go.sum ./
RUN go mod download

COPY tee-relay-client .

RUN go build -o ./tee-relay-client cmd/main/main.go

FROM debian:trixie@sha256:fd8f5a1df07b5195613e4b9a0b6a947d3772a151b81975db27d47f093f60c6e6 AS execution

WORKDIR /app

COPY --from=builder /app/tee-relay-client/tee-relay-client .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD ["./tee-relay-client" ]

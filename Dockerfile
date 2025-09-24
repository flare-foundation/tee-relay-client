FROM golang:1.25.1 AS builder

WORKDIR /app
COPY tee-node ./tee-node

WORKDIR /app/tee-relay-client
COPY tee-relay-client/go.mod tee-relay-client/go.sum ./
RUN go mod download

COPY tee-relay-client .

RUN go build -o ./tee-relay-client cmd/testmain/main.go

FROM debian:latest AS execution

WORKDIR /app

COPY --from=builder /app/tee-relay-client/tee-relay-client .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD ["./tee-relay-client" ]

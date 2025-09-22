FROM golang:1.25.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /app/tee-relay-client cmd/testmain/main.go

FROM debian:latest AS execution

WORKDIR /app

COPY --from=builder /app/tee-relay-client .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD ["./tee-relay-client" ]

FROM golang:1.24.4 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /app/tee-relay-client main/main.go

FROM debian:latest AS execution

WORKDIR /app

COPY --from=builder /app/tee-relay-client .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD ["./tee-relay-client" ]

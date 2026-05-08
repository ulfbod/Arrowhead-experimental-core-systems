# Builds the consumer-direct-tls service for experiment-7.
# AMQP consumer using TLS (amqps://) with full AHC orchestration flow.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-7/services/consumer-direct-tls/ ./experiments/experiment-7/services/consumer-direct-tls/
WORKDIR /src/experiments/experiment-7/services/consumer-direct-tls
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

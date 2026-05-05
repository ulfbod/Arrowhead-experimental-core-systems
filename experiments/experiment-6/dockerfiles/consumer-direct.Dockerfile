# Builds the consumer-direct service for experiment-6 (full AHC orchestration flow, AMQP path).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-5/services/consumer-direct/ ./experiments/experiment-5/services/consumer-direct/
WORKDIR /src/experiments/experiment-5/services/consumer-direct
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

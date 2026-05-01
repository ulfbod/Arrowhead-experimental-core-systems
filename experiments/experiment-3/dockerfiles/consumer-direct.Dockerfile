# Builds the consumer-direct service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-3/services/consumer-direct/ ./experiments/experiment-3/services/consumer-direct/
WORKDIR /src/experiments/experiment-3/services/consumer-direct
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

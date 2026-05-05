# Builds the analytics-consumer service for experiment-6 (Kafka SSE path).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY experiments/experiment-5/services/analytics-consumer/ ./experiments/experiment-5/services/analytics-consumer/
WORKDIR /src/experiments/experiment-5/services/analytics-consumer
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

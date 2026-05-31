# Builds the data-provider-tls service for experiment-7.
# HTTPS server + TLS Kafka consumer. Gets its own cert from the core CA.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY experiments/experiment-7/services/data-provider-tls/ ./experiments/experiment-7/services/data-provider-tls/
WORKDIR /src/experiments/experiment-7/services/data-provider-tls
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

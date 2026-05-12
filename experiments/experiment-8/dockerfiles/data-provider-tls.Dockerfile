# Builds the data-provider-tls service for experiment-8.
# HTTPS server + TLS Kafka consumer. Gets its own cert via AH5.2 PKI lifecycle.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY experiments/experiment-8/services/data-provider-tls/ ./experiments/experiment-8/services/data-provider-tls/
WORKDIR /src/experiments/experiment-8/services/data-provider-tls
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
EXPOSE 9094
ENTRYPOINT ["/app"]

# Builds the data-provider service for experiment-6 (Kafka consumer + REST API).
# rest-authz proxies authorized requests to this service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY experiments/experiment-6/services/data-provider/ ./experiments/experiment-6/services/data-provider/
WORKDIR /src/experiments/experiment-6/services/data-provider
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

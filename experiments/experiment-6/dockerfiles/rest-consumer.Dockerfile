# Builds the rest-consumer service for experiment-6 (REST client via rest-authz PEP).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY experiments/experiment-6/services/rest-consumer/ ./experiments/experiment-6/services/rest-consumer/
WORKDIR /src/experiments/experiment-6/services/rest-consumer
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

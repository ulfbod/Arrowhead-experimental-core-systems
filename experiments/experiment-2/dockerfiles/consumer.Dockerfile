# Builds the consumer service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src

# Copy service source.
COPY experiments/experiment-2/services/consumer/ ./services/consumer/

WORKDIR /src/services/consumer
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

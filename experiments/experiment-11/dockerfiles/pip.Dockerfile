# Builds the PIP (Policy Information Point) service for experiment-11.
# PIP polls ConsumerAuth and caches a versioned grant table.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-11/services/pip
COPY experiments/experiment-11/services/pip/ .
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9406
ENTRYPOINT ["/app"]

# Builds the PIP (Policy Information Point / subject attribute registry) for experiment-12.
# Reuses the experiment-10 PIP service (cert-level attribute store, no grant sync).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-10/services/pip
COPY experiments/experiment-10/services/pip/ .
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9506
ENTRYPOINT ["/app"]

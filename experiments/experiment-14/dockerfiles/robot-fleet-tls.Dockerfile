# Builds the robot-fleet-tls service for experiment-14.
# Unchanged from experiment-13 — reuses experiment-13 robot-fleet-tls source.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-13/services/robot-fleet-tls/ ./experiments/experiment-13/services/robot-fleet-tls/
WORKDIR /src/experiments/experiment-13/services/robot-fleet-tls
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the robot-fleet-tls service for experiment-13.
# Uses experiment-13's own copy of robot-fleet-tls which adds mTLS client cert
# to the AMQPS connection (required by experiment-13 RabbitMQ fail_if_no_peer_cert).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-13/services/robot-fleet-tls/ ./experiments/experiment-13/services/robot-fleet-tls/
WORKDIR /src/experiments/experiment-13/services/robot-fleet-tls
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the robot-fleet service (reused from experiment-2).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-2/services/robot-fleet/ ./experiments/experiment-2/services/robot-fleet/
WORKDIR /src/experiments/experiment-2/services/robot-fleet
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the robot-simulator service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src

# Copy message-broker dependency first (used by replace directive).
COPY support/message-broker/ ./support/message-broker/

# Copy service source.
COPY experiments/experiment-2/services/robot-simulator/ ./services/robot-simulator/

WORKDIR /src/services/robot-simulator
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the robot-fleet service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src

# Copy message-broker dependency first (used by replace directive).
COPY support/message-broker/ ./support/message-broker/

# Copy service source, preserving the repo-relative path so the replace
# directive (../../../../support/message-broker) resolves correctly.
COPY experiments/experiment-2/services/robot-fleet/ ./experiments/experiment-2/services/robot-fleet/

WORKDIR /src/experiments/experiment-2/services/robot-fleet
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

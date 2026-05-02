# Builds the robot-fleet service for experiment-4 (SR registration + Auth login).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-4/services/robot-fleet/ ./experiments/experiment-4/services/robot-fleet/
WORKDIR /src/experiments/experiment-4/services/robot-fleet
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

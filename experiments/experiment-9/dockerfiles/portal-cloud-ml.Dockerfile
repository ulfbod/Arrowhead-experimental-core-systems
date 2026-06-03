# Builds the portal-cloud-ml service for experiment-9.
# UC3 Portal & Cloud ML: aggregates robot telemetry via kafka-authz SSE
# and direct AMQP subscription; serves HTTPS REST API for service partners.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/message-broker/ ./support/message-broker/
COPY experiments/experiment-9/services/portal-cloud-ml/ ./experiments/experiment-9/services/portal-cloud-ml/
WORKDIR /src/experiments/experiment-9/services/portal-cloud-ml
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9207 9294
ENTRYPOINT ["/app"]

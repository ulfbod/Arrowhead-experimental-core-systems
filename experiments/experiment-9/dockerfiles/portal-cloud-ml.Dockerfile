# Builds the portal-cloud-ml service for experiment-9.
# UC3 Portal & Cloud ML: aggregates robot telemetry via kafka-authz SSE,
# serves HTTPS REST API for service partners via pki-rest-authz.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-9/services/portal-cloud-ml
COPY experiments/experiment-9/services/portal-cloud-ml/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9207 9294
ENTRYPOINT ["/app"]

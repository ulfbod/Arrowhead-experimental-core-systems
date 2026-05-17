# Builds the topic-auth-xacml service for experiment-14.
# Extended with connection-time cert-validity pre-gate (D2'): before consulting
# AuthzForce, handleUser and handleVhost query PIP directly. If certValid=false,
# the AMQP connection is rejected without calling AuthzForce at all.
# Identity comes from RabbitMQ username = cert CN (ssl_cert_login plugin).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-14/services/topic-auth-xacml/ /build/experiments/experiment-14/services/topic-auth-xacml/
WORKDIR /build/experiments/experiment-14/services/topic-auth-xacml
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the topic-auth-xacml service for experiment-13.
# Extended with PIP cert-level enrichment: before each AuthzForce decision,
# queries PIP for {certLevel, valid} and includes them as XACML subject attributes.
# Identity comes from RabbitMQ username = cert CN (ssl_cert_login plugin).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-13/services/topic-auth-xacml/ /build/experiments/experiment-13/services/topic-auth-xacml/
WORKDIR /build/experiments/experiment-13/services/topic-auth-xacml
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

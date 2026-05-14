# Builds the kafka-authz service for experiment-13.
# Extended with PIP cert-level enrichment: before each AuthzForce decision,
# queries PIP for {certLevel, valid} and includes them as XACML subject attributes.
# Identity comes from Kafka principal (cert CN — ssl.client.auth=required).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-13/services/kafka-authz/ /build/experiments/experiment-13/services/kafka-authz/
WORKDIR /build/experiments/experiment-13/services/kafka-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9101
ENTRYPOINT ["/app"]

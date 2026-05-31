# Builds the kafka-authz service for experiment-7 (Kafka SSE PEP → AuthzForce).
# Includes optional TLS support for Kafka broker connections when CA_URL is set.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/kafka-authz/ ./support/kafka-authz/
WORKDIR /src/support/kafka-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

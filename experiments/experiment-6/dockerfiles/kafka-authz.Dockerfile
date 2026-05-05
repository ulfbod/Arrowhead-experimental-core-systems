# Builds the kafka-authz service (Kafka SSE proxy with AuthzForce enforcement).
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/kafka-authz/ ./support/kafka-authz/
WORKDIR /src/support/kafka-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

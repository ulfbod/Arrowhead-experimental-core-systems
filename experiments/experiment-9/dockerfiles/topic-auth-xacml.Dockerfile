# Builds the topic-auth-xacml service for experiment-9.
# RabbitMQ HTTP auth backend backed by AuthzForce XACML.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/topic-auth-xacml/ ./support/topic-auth-xacml/
WORKDIR /src/support/topic-auth-xacml
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

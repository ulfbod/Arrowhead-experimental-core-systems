# Builds the cert-consumer service for experiment-7.
# Issues an X.509 certificate from the core CA and uses it to authenticate
# via mTLS to cert-rest-authz, demonstrating certificate-based identity.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY experiments/experiment-7/services/cert-consumer/ ./experiments/experiment-7/services/cert-consumer/
WORKDIR /src/experiments/experiment-7/services/cert-consumer
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

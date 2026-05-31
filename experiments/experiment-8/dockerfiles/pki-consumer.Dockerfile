# Builds the pki-consumer service for experiment-8.
# Polls pki-rest-authz using full Arrowhead 5.2 PKI lifecycle for mTLS identity.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY experiments/experiment-8/services/pki-consumer/ ./experiments/experiment-8/services/pki-consumer/
WORKDIR /src/experiments/experiment-8/services/pki-consumer
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
EXPOSE 9107
ENTRYPOINT ["/app"]

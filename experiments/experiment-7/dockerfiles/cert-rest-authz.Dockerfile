# Builds the cert-rest-authz service for experiment-7.
# mTLS-aware REST reverse-proxy PEP: reads consumer identity from client
# certificate CN instead of X-Consumer-Name header.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY experiments/experiment-7/services/cert-rest-authz/ ./experiments/experiment-7/services/cert-rest-authz/
WORKDIR /src/experiments/experiment-7/services/cert-rest-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

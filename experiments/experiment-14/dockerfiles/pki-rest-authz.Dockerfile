# Builds the pki-rest-authz service for experiment-14.
# Unchanged from experiment-13 — reuses experiment-13 pki-rest-authz source.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-13/services/pki-rest-authz/ /build/experiments/experiment-13/services/pki-rest-authz/
WORKDIR /build/experiments/experiment-13/services/pki-rest-authz
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9208 9209
ENTRYPOINT ["/app"]

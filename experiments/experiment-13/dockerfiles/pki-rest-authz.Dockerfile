# Builds the pki-rest-authz service for experiment-13.
# Extended with PIP cert-level enrichment: after extracting cert CN from mTLS,
# queries PIP for {certLevel, valid} and includes them as XACML subject attributes.
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

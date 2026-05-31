# Builds the pki-rest-authz service for experiment-10.
# mTLS reverse-proxy PEP backed by AuthzForce XACML.
# Reuses experiment-8 pki-rest-authz source unchanged.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /build/experiments/experiment-8/services/pki-rest-authz
COPY support/ /build/support/
COPY experiments/experiment-8/services/pki-rest-authz/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9208 9209
ENTRYPOINT ["/app"]

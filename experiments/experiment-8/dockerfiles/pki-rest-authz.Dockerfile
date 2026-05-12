# Builds the pki-rest-authz service for experiment-8.
# mTLS reverse-proxy PEP backed by AuthzForce XACML.
# Uses Arrowhead 5.2 PKI lifecycle for identity acquisition.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-8/services/pki-rest-authz
COPY support/ /build/support/
COPY experiments/experiment-8/services/pki-rest-authz/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9108 9109
ENTRYPOINT ["/app"]

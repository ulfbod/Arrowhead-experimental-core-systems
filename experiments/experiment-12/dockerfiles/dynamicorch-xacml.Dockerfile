# Builds the DynamicOrchestration-XACML service (core-evol/cmd/dynamicorch-xacml).
#
# This is the AH5-evolved DynamicOrch that calls authz-pdp over gRPC
# (authorize.proto) instead of ConsumerAuth.verify.
#
# Auth backends (AUTH_BACKEND env var):
#   grpc         — calls authz-pdp gRPC service (default, Approach B)
#   consumerauth — calls AH5 ConsumerAuthorization (for comparison)
#
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY core-evol/ /build/core-evol/
WORKDIR /build/core-evol
RUN CGO_ENABLED=0 go build -o /app ./cmd/dynamicorch-xacml

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8083
ENTRYPOINT ["/app"]

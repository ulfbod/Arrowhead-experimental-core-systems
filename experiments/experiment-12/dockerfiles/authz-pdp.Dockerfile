# Builds the authz-pdp gRPC server (core-evol/cmd/authz-pdp).
#
# authz-pdp implements the AuthorizationPDP service (proto/authorize/authorize.proto).
# It translates gRPC DecisionRequests to XACML 3.0 requests with separate
# resource-id (service) and provider-id attributes, then evaluates them
# against AuthzForce CE.
#
# gRPC reflection is enabled for grpcurl introspection.
#
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY support/authzforce/ /build/support/authzforce/
COPY core-evol/ /build/core-evol/
WORKDIR /build/core-evol
RUN CGO_ENABLED=0 go build -o /app ./cmd/authz-pdp

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9550
ENTRYPOINT ["/app"]

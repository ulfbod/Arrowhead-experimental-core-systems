# Builds the DynamicOrchestration-XACML service (core-evol).
# This is the AH5-evolved DynamicOrch that replaces ConsumerAuth.verify with
# a single AuthzForce XACML decision (Approach B — see AH5_EVOL.md).
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

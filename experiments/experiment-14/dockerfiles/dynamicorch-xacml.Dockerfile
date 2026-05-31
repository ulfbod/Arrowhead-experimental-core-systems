# Builds the DynamicOrch-XACML service for experiment-14.
# Unchanged from experiment-13 — reuses core-evol/cmd/dynamicorch-xacml.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
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

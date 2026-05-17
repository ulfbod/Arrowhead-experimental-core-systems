# Builds the authz-pdp gRPC server for experiment-14.
# Unchanged from experiment-13 — reuses core-evol/cmd/authz-pdp.
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

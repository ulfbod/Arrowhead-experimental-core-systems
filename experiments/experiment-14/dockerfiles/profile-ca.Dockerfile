# Builds the profile-ca service for experiment-14.
# Unchanged from experiment-13 — reuses experiment-13 profile-ca source.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /build
# core-evol contains proto/certlifecycle generated Go code
COPY core-evol/ /build/core-evol/
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-13/services/profile-ca/ /build/experiments/experiment-13/services/profile-ca/
WORKDIR /build/experiments/experiment-13/services/profile-ca
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8087 8088 8089
ENTRYPOINT ["/app"]

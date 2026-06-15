# Builds the profile-ca service for experiment-15.
# Experiment-15: CA-as-PIP — profile-ca includes PIP endpoints.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /build
# core-evol contains proto/certlifecycle generated Go code
COPY core-evol/ /build/core-evol/
COPY support/authzforce/ /build/support/authzforce/
COPY experiments/experiment-15/services/profile-ca/ /build/experiments/experiment-15/services/profile-ca/
WORKDIR /build/experiments/experiment-15/services/profile-ca
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8787 8788 8789
ENTRYPOINT ["/app"]

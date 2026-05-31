# Builds the policy-sync service for experiment-7.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/policy-sync/ ./support/policy-sync/
WORKDIR /src/support/policy-sync
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

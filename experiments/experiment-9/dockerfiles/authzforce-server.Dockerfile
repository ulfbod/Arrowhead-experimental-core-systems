# Authzforce server for experiment-9 — reuses support/authzforce-server.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/authzforce-server/ ./support/authzforce-server/
WORKDIR /src/support/authzforce-server
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]

# Builds the topic-auth-sync service.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/topic-auth-sync/ ./support/topic-auth-sync/
WORKDIR /src/support/topic-auth-sync
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

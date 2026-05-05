# Builds the authzforce-server — lightweight XACML PDP/PAP server.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/authzforce-server/ ./support/authzforce-server/
WORKDIR /src/support/authzforce-server
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

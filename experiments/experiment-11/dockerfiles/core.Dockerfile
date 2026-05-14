# Builds an Arrowhead core system binary for experiment-10.
# Same as experiment-8 core.Dockerfile — reuses the core module directly.
# Build context: repo root (ArrowheadCore/core/)

FROM golang:1.22-alpine AS builder
ARG CMD
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/${CMD}

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

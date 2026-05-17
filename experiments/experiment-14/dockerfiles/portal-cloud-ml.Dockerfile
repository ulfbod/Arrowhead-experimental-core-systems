# Builds the portal-cloud-ml service for experiment-14.
# Unchanged from experiment-13 — reuses experiment-9 portal-cloud-ml source.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-9/services/portal-cloud-ml
COPY experiments/experiment-9/services/portal-cloud-ml/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9207 9294
ENTRYPOINT ["/app"]

# Builds the service-partner service for experiment-13.
# Reuses experiment-10 service-partner source unchanged.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-10/services/service-partner
COPY experiments/experiment-10/services/service-partner/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9211
ENTRYPOINT ["/app"]

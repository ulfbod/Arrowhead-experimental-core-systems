# Builds the AuthzForce CE server wrapper for experiment-7.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY support/authzforce/ ./support/authzforce/
COPY support/authzforce-server/ ./support/authzforce-server/
WORKDIR /src/support/authzforce-server
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM eclipse-temurin:17-jre-alpine
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

# Builds the profile-ca service for experiment-10.
# Arrowhead 5.2 Local Cloud CA with profile hierarchy enforcement.
# Reuses experiment-8 profile-ca source unchanged.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /src/experiments/experiment-8/services/profile-ca
COPY experiments/experiment-8/services/profile-ca/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 8187 8188
ENTRYPOINT ["/app"]

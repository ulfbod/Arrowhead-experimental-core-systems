# Builds the cert-provisioner for experiment-8.
# Issues X.509 certificates from the profile-ca for infrastructure services
# (Kafka, RabbitMQ) and writes them to a shared Docker volume.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY experiments/experiment-8/services/cert-provisioner/ ./experiments/experiment-8/services/cert-provisioner/
WORKDIR /src/experiments/experiment-8/services/cert-provisioner
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

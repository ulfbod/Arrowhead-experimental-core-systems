# Builds the topic-auth-sync service.
# Build context: repo root (ArrowheadCore/)
#
# NOTE: This Dockerfile exists because topic-auth-sync was considered as an
# authorization mechanism for experiment-4 during design (see AHC-INTEGRATION-PLAN.md),
# but the experiment ultimately uses topic-auth-http instead. The file is retained
# for reference. topic-auth-sync is NOT wired into experiment-4/docker-compose.yml.

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY support/topic-auth-sync/ ./support/topic-auth-sync/
WORKDIR /src/support/topic-auth-sync
RUN CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
ENTRYPOINT ["/app"]

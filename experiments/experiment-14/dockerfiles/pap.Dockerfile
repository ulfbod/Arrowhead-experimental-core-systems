# Builds the PAP (Policy Administration Point) for experiment-14.
# Reuses experiment-10 PAP service unchanged.
# Build context: repo root (ArrowheadCore/)

FROM golang:1.22-alpine AS builder
WORKDIR /build/experiments/experiment-10/services/pap
COPY support/ /build/support/
COPY experiments/experiment-10/services/pap/ .
RUN go mod download && CGO_ENABLED=0 go build -o /app .

FROM alpine:3.19
RUN apk add --no-cache wget
COPY --from=builder /app /app
EXPOSE 9505
ENTRYPOINT ["/app"]

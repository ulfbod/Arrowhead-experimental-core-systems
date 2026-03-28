FROM golang:1.21-alpine AS builder
ARG SERVICE_DIR
WORKDIR /app
COPY go.mod ./
RUN go mod download || true
COPY . .
RUN go build -o /app/service ./cmd/${SERVICE_DIR}

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/service .
EXPOSE 8000
CMD ["./service"]

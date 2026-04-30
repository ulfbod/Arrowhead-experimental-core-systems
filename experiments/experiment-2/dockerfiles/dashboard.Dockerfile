# Builds the experiment-2 dashboard (React) and serves it with nginx.
# Build context: repo root (ArrowheadCore/)

# Stage 1: build the React app.
FROM node:20-alpine AS builder
WORKDIR /app
COPY experiments/experiment-2/dashboard/package*.json ./
RUN npm ci
COPY experiments/experiment-2/dashboard/ ./
RUN npm run build

# Stage 2: serve with nginx + reverse proxy to core services.
FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY experiments/experiment-2/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

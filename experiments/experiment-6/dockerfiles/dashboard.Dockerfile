# Builds the experiment-6 dashboard (React + Vite → nginx).
# Build context: repo root (ArrowheadCore/)
#
# Symlinks in dashboard/src/ point to support/dashboard-shared/.
# Docker copies relative symlinks as-is; inside the container those relative
# paths are dangling. We therefore:
#   1. COPY the dashboard (symlinks arrive as dangling symlinks)
#   2. COPY support/dashboard-shared/ to a temp location
#   3. Remove the dangling symlinks and replace them with the actual files
# See EXPERIENCES.md EXP-010.

FROM node:20-alpine AS build
WORKDIR /app
COPY experiments/experiment-6/dashboard/package.json ./
RUN npm install
COPY experiments/experiment-6/dashboard/ .
COPY support/dashboard-shared/ /dashboard-shared/
RUN find src -type l -delete && cp -r /dashboard-shared/. src/
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY experiments/experiment-6/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

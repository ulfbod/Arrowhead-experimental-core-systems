# Builds the experiment-9 dashboard (static HTML + nginx).
# Build context: repo root (ArrowheadCore/)

FROM nginx:alpine
COPY experiments/experiment-9/dashboard/index.html /usr/share/nginx/html/index.html
COPY experiments/experiment-9/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

# Builds the experiment-12 dashboard (static HTML + nginx).
# Build context: repo root (ArrowheadCore/)

FROM nginx:alpine
COPY experiments/experiment-12/dashboard/index.html /usr/share/nginx/html/index.html
COPY experiments/experiment-12/dashboard/admin.html /usr/share/nginx/html/admin.html
COPY experiments/experiment-12/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

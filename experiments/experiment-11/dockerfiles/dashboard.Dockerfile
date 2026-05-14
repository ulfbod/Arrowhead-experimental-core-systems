# Builds the experiment-11 dashboard (static HTML + nginx).
# Serves both monitoring (index.html) and policy admin (admin.html).
# Build context: repo root (ArrowheadCore/)

FROM nginx:alpine
COPY experiments/experiment-11/dashboard/index.html /usr/share/nginx/html/index.html
COPY experiments/experiment-11/dashboard/admin.html /usr/share/nginx/html/admin.html
COPY experiments/experiment-11/dashboard/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80

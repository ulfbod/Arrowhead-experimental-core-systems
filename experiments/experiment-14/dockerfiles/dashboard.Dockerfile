# Experiment-14 dashboard (static HTML + nginx).
# Build context: repo root (ArrowheadCore/)

FROM nginx:alpine
COPY experiments/experiment-14/dashboard/index.html /usr/share/nginx/html/index.html
COPY experiments/experiment-14/dashboard/admin.html  /usr/share/nginx/html/admin.html
COPY experiments/experiment-14/dashboard/demo.html   /usr/share/nginx/html/demo.html
COPY experiments/experiment-14/dashboard/map-bg.png   /usr/share/nginx/html/map-bg.png
COPY experiments/experiment-14/dashboard/ah-logo.png  /usr/share/nginx/html/ah-logo.png
COPY experiments/experiment-14/dashboard/nginx.conf  /etc/nginx/conf.d/default.conf
EXPOSE 80

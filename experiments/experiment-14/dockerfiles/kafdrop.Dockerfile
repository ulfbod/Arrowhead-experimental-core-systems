# Kafdrop Dockerfile for experiment-14.
# Extends the official Kafdrop image with a startup script that generates
# kafka.properties using PEM certs from the shared certs volume (mTLS Kafka).
# Build context: repo root (ArrowheadCore/)

FROM obsidiandynamics/kafdrop:latest

COPY experiments/experiment-14/kafdrop/entrypoint.sh /kafdrop-entrypoint.sh
RUN chmod +x /kafdrop-entrypoint.sh

ENTRYPOINT ["/kafdrop-entrypoint.sh"]

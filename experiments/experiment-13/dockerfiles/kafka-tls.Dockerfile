# Kafka with TLS + client authentication for experiment-13.
# ssl.client.auth=required enforces mTLS — clients must present a valid cert.
# Build context: repo root (ArrowheadCore/)

FROM confluentinc/cp-kafka:7.6.1
USER root
RUN microdnf install -y openssl
COPY experiments/experiment-13/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1000
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]

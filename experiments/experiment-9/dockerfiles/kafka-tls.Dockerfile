# Kafka with TLS for experiment-9.
# Reuses experiment-8 kafka-tls pattern unchanged.
# Build context: repo root (ArrowheadCore/)

FROM confluentinc/cp-kafka:7.6.1
USER root
RUN microdnf install -y openssl
COPY experiments/experiment-9/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1000
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]

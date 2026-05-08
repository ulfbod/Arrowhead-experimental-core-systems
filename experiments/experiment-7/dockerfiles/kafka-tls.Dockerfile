# Kafka broker with TLS (SSL) support for experiment-7.
# Converts PEM certificates (written by cert-provisioner) to PKCS12 keystores
# at startup, then starts the Confluent Kafka broker with SSL listeners.

FROM confluentinc/cp-kafka:7.6.1
USER root
RUN apt-get update -qq && apt-get install -y --no-install-recommends openssl && rm -rf /var/lib/apt/lists/*
COPY experiments/experiment-7/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1001
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]

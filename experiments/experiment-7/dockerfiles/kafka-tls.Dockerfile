# Kafka broker with TLS (SSL) support for experiment-7.
# Converts PEM certificates (written by cert-provisioner) to PKCS12 keystores
# at startup, then starts the Confluent Kafka broker with SSL listeners.

# confluentinc/cp-kafka:7.6.1 is RHEL 8-based and ships openssl at /usr/bin/openssl.
# No additional package installation is needed.
FROM confluentinc/cp-kafka:7.6.1
USER root
COPY experiments/experiment-7/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh
USER 1001
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]

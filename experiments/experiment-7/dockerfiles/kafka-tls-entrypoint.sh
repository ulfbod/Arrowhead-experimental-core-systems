#!/bin/bash
# kafka-tls-entrypoint.sh — Prepares PKCS12 keystores from PEM cert files
# and starts the Confluent Kafka broker with SSL enabled.
#
# This entrypoint:
#  1. Waits for cert files to appear in CERT_DIR (written by cert-provisioner)
#  2. Converts PEM → PKCS12 using openssl
#  3. Sets Kafka SSL environment variables
#  4. Delegates to the original Confluent entrypoint

set -e

CERT_DIR="${CERT_DIR:-/certs}"
KAFKA_KEYSTORE_PASS="${KAFKA_KEYSTORE_PASS:-kafkapass123}"
KEYSTORE_PATH="/tmp/kafka-keystore.p12"
TRUSTSTORE_PATH="/tmp/kafka-truststore.p12"

echo "[kafka-tls] waiting for cert files in $CERT_DIR..."
for i in $(seq 1 30); do
  if [ -f "$CERT_DIR/kafka.crt" ] && [ -f "$CERT_DIR/kafka.key" ] && [ -f "$CERT_DIR/ca.crt" ]; then
    echo "[kafka-tls] cert files found"
    break
  fi
  echo "[kafka-tls] attempt $i/30 — not ready, sleeping 2s"
  sleep 2
done

if [ ! -f "$CERT_DIR/kafka.crt" ]; then
  echo "[kafka-tls] ERROR: cert files not found in $CERT_DIR after 60s" >&2
  exit 1
fi

# Build PKCS12 keystore (broker cert + key)
openssl pkcs12 -export \
  -in  "$CERT_DIR/kafka.crt" \
  -inkey "$CERT_DIR/kafka.key" \
  -out "$KEYSTORE_PATH" \
  -passout "pass:$KAFKA_KEYSTORE_PASS" \
  -name kafka 2>/dev/null
echo "[kafka-tls] keystore created: $KEYSTORE_PATH"

# Build PKCS12 truststore (CA cert only)
openssl pkcs12 -export \
  -nokeys \
  -in  "$CERT_DIR/ca.crt" \
  -out "$TRUSTSTORE_PATH" \
  -passout "pass:$KAFKA_KEYSTORE_PASS" 2>/dev/null
echo "[kafka-tls] truststore created: $TRUSTSTORE_PATH"

# Configure Kafka SSL via environment variables (picked up by Confluent CP start)
export KAFKA_SSL_KEYSTORE_TYPE=PKCS12
export KAFKA_SSL_KEYSTORE_LOCATION="$KEYSTORE_PATH"
export KAFKA_SSL_KEYSTORE_PASSWORD="$KAFKA_KEYSTORE_PASS"
export KAFKA_SSL_TRUSTSTORE_TYPE=PKCS12
export KAFKA_SSL_TRUSTSTORE_LOCATION="$TRUSTSTORE_PATH"
export KAFKA_SSL_TRUSTSTORE_PASSWORD="$KAFKA_KEYSTORE_PASS"

echo "[kafka-tls] SSL configured — starting Kafka"
exec /etc/confluent/docker/run

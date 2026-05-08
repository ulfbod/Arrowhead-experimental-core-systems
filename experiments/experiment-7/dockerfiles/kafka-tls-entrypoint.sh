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
# Confluent's dub config tool resolves KAFKA_SSL_KEYSTORE_FILENAME relative to
# /etc/kafka/secrets/ — keystores must live there, not in /tmp.
SECRETS_DIR="/etc/kafka/secrets"
KEYSTORE_PATH="$SECRETS_DIR/kafka-keystore.p12"
TRUSTSTORE_PATH="$SECRETS_DIR/kafka-truststore.jks"
mkdir -p "$SECRETS_DIR"

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

# Remove any stale keystores from a previous container run (container may be
# restarted rather than recreated, so the filesystem is not guaranteed clean).
rm -f "$KEYSTORE_PATH" "$TRUSTSTORE_PATH"

# Build PKCS12 keystore (broker cert + key)
openssl pkcs12 -export \
  -in  "$CERT_DIR/kafka.crt" \
  -inkey "$CERT_DIR/kafka.key" \
  -out "$KEYSTORE_PATH" \
  -passout "pass:$KAFKA_KEYSTORE_PASS" \
  -name kafka 2>/dev/null
echo "[kafka-tls] keystore created: $KEYSTORE_PATH"

# Build JKS truststore (CA cert only) using keytool.
# openssl pkcs12 does not set the trusted-key-usage attribute on the CA entry;
# Java's SSL engine requires it. keytool -importcert -trustcacerts sets it correctly.
# JKS is used (not PKCS12) because Java's SSL stack reads JKS trusted entries
# more reliably than PKCS12 trusted entries across different Java/OpenSSL versions.
/usr/lib/jvm/jre/bin/keytool -importcert \
  -alias ca \
  -file "$CERT_DIR/ca.crt" \
  -keystore "$TRUSTSTORE_PATH" \
  -storetype JKS \
  -storepass "$KAFKA_KEYSTORE_PASS" \
  -noprompt 2>/dev/null
echo "[kafka-tls] truststore created: $TRUSTSTORE_PATH"

# dub also requires SSL passwords to be supplied as plaintext credential files
# in /etc/kafka/secrets/, referenced by bare filename via KAFKA_SSL_*_CREDENTIALS.
# KAFKA_SSL_KEY_CREDENTIALS     — private key password (same as keystore pass for PKCS12)
# KAFKA_SSL_KEYSTORE_CREDENTIALS  — keystore password
# KAFKA_SSL_TRUSTSTORE_CREDENTIALS — truststore password
printf '%s' "$KAFKA_KEYSTORE_PASS" > "$SECRETS_DIR/kafka_keystore_creds"
printf '%s' "$KAFKA_KEYSTORE_PASS" > "$SECRETS_DIR/kafka_key_creds"
echo "[kafka-tls] credential files written"

# Configure Kafka SSL via environment variables (picked up by Confluent CP / dub).
# dub requires *_FILENAME (bare name relative to /etc/kafka/secrets/), not *_LOCATION.
# Passwords come from credential files, not from *_PASSWORD vars directly.
export KAFKA_SSL_KEYSTORE_TYPE=PKCS12
export KAFKA_SSL_KEYSTORE_FILENAME="kafka-keystore.p12"
export KAFKA_SSL_KEYSTORE_CREDENTIALS="kafka_keystore_creds"
export KAFKA_SSL_TRUSTSTORE_TYPE=JKS
# dub translates KAFKA_SSL_KEYSTORE_FILENAME → ssl.keystore.location but does NOT
# perform the same translation for the truststore. Use the absolute LOCATION var instead.
export KAFKA_SSL_TRUSTSTORE_LOCATION="$TRUSTSTORE_PATH"
export KAFKA_SSL_TRUSTSTORE_PASSWORD="$KAFKA_KEYSTORE_PASS"
export KAFKA_SSL_KEY_CREDENTIALS="kafka_key_creds"
unset KAFKA_SSL_TRUSTSTORE_FILENAME    # ensure no stale _FILENAME var interferes

echo "[kafka-tls] SSL configured — starting Kafka"
exec /etc/confluent/docker/run

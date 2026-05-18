#!/bin/sh
# Kafdrop entrypoint for experiment-14.
#
# Converts the EC private key from SEC1 to PKCS#8 (required by Java's PEM parser),
# then passes SSL config via KAFKA_PROPERTIES base64 + file:// CMD_ARGS.
#
# Why SEC1 → PKCS#8 conversion:
#   The cert-provisioner issues EC keys in SEC1 format (BEGIN EC PRIVATE KEY).
#   Java's KeyFactory only handles PKCS#8 (BEGIN PRIVATE KEY); SEC1 throws
#   InvalidKeyException: "algid parse error, not a sequence". See EXP-031.
#
# Why KAFKA_PROPERTIES + file:// (not --kafka.properties.<key>=<val> CLI args):
#   Spring CLI args for Map/Properties fields lose dotted keys at binding time;
#   the Kafka client never sees security.protocol=SSL. And bare absolute paths
#   are resolved as ClassPathResources (not found). file:// prefix gives a
#   FileUrlResource that Spring can read. See EXP-031.
set -e

CERTS="${CERTS_DIR:-/certs}"

echo "[kafdrop] Converting EC private key from SEC1 to PKCS#8 (Java compatibility)..."
# Java's PEM loader requires PKCS#8 (BEGIN PRIVATE KEY), not SEC1 (BEGIN EC PRIVATE KEY).
openssl pkcs8 -topk8 -nocrypt -in "$CERTS/kafka.key" -out /tmp/kafka-pkcs8.key
cat "$CERTS/kafka.crt" /tmp/kafka-pkcs8.key > /tmp/kafka-client.pem

echo "[kafdrop] Writing Kafka SSL properties via KAFKA_PROPERTIES (base64)..."
# /kafdrop.sh decodes KAFKA_PROPERTIES into KAFKA_PROPERTIES_FILE before java starts.
PROPS=$(printf '%s\n' \
  "security.protocol=SSL" \
  "ssl.truststore.type=PEM" \
  "ssl.truststore.location=${CERTS}/ca.crt" \
  "ssl.keystore.type=PEM" \
  "ssl.keystore.location=/tmp/kafka-client.pem" \
  "ssl.endpoint.identification.algorithm=" \
)
export KAFKA_PROPERTIES=$(printf '%s' "$PROPS" | base64 | tr -d '\n')
export KAFKA_PROPERTIES_FILE=/tmp/kafka.properties

echo "[kafdrop] Starting Kafdrop..."
# file:// prefix: Spring's DefaultResourceLoader returns FileUrlResource (not ClassPathResource).
export CMD_ARGS="--kafka.brokerConnect=kafka:9092 --kafka.properties.file=file:///tmp/kafka.properties --server.port=9000"

exec /kafdrop.sh

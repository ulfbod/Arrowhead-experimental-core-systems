# kafka-principal-builder.Dockerfile — Multi-stage build for the Arrowhead
# KafkaPrincipalBuilder plugin (experiment-14).
#
# Stage 1: Maven build — compiles Java sources, runs tests, assembles fat JAR.
# Stage 2: cp-kafka image — copies the plugin JAR to /opt/kafka-plugins/.
#
# The fat JAR bundles jackson-databind so the plugin is self-contained.
# kafka-clients is provided-scope (available from the broker classpath).
#
# Build context: repo root (ArrowheadCore/)

# ── Stage 1: Maven build ──────────────────────────────────────────────────────
FROM maven:3.9.6-eclipse-temurin-11 AS builder

WORKDIR /build
COPY experiments/experiment-14/services/kafka-principal-builder/pom.xml .
# Download dependencies first (layer cache — invalidated only when pom.xml changes)
RUN mvn dependency:go-offline -q

COPY experiments/experiment-14/services/kafka-principal-builder/src/ ./src/
RUN mvn package -q

# ── Stage 2: cp-kafka base image ──────────────────────────────────────────────
FROM confluentinc/cp-kafka:7.6.1

COPY --from=builder /build/target/kafka-principal-builder.jar /opt/kafka-plugins/kafka-principal-builder.jar

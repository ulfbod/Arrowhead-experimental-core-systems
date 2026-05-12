# DIAGRAMS.md — Experiment 8

ASCII-art architectural diagrams for experiment-8.
For Mermaid versions see DIAGRAMS_MERMAID_BASICS.md and DIAGRAMS_MERMAID_SECURITY.md.

---

## 1. Full System Overview

```
  ╔══════════════════════════════════════════════════════════════════════════════╗
  ║  Experiment 8 — Arrowhead 5.2 Profile-Based PKI                            ║
  ╚══════════════════════════════════════════════════════════════════════════════╝

  ┌──────────────────────────────────────────────────────────────────────┐
  │  Arrowhead Core (TLS-secured, mTLS inter-service)                    │
  │                                                                      │
  │  ServiceRegistry :8490   Authentication :8491                        │
  │  ConsumerAuth    :8492   DynamicOrch    :8493                        │
  └──────────────────────────────────────────────────────────────────────┘
          │ grants lookup (mTLS)        │ auth/orch queries
          ▼                             ▼
  ┌──────────────────┐        ┌──────────────────────┐
  │  profile-ca      │        │  AuthzForce PDP/PAP  │
  │  HTTP  :8087     │        │  port 8080 (internal) │
  │  mTLS  :8088     │        │  domain:arrowhead-exp8│
  └──────────────────┘        └──────────────────────┘
          │                             │
          │ cert issuance               │ XACML decisions
          ▼                             ▼
  ┌──────────────────┐        ┌──────────────────────┐
  │  cert-provisioner│        │  policy-sync :9105   │
  │  (one-shot)      │        │  (polls ConsumerAuth,│
  │  writes /certs   │        │   uploads PolicySet) │
  └──────────────────┘        └──────────────────────┘
          │                             │
          ▼                             │
  /certs volume                         │
  (ca.crt, infra certs)                 │
          │              ┌──────────────┤──────────────┐
          │              │              │              │
          ▼              ▼              ▼              ▼
  ┌──────────┐  ┌──────────────┐ ┌──────────┐ ┌────────────────┐
  │  Kafka   │  │topic-auth-   │ │kafka-    │ │pki-rest-authz  │
  │  SSL     │  │xacml :9090   │ │authz     │ │mTLS :9108      │
  │  :9092   │  │(RabbitMQ PEP)│ │:9101     │ │HTTP :9109      │
  └──────────┘  └──────────────┘ │(Kafka PEP│ │(REST PEP)      │
       │               │         └──────────┘ └────────────────┘
       │               │              │              │
       ▼               ▼              ▼              ▼
  ┌──────────┐  ┌──────────────┐ ┌──────────┐ ┌────────────────┐
  │RabbitMQ  │  │consumer-1/2/3│ │analytics-│ │data-provider   │
  │AMQPS 5671│  │(AMQP)        │ │consumer  │ │-tls :9094      │
  └──────────┘  └──────────────┘ │(SSE)     │ │(HTTPS/Kafka)   │
       ▲                          └──────────┘ └────────────────┘
       │                                              ▲
  robot-fleet-tls :9003                               │
  (publishes AMQP + Kafka)                     pki-consumer :9107
                                               (mTLS REST, OU=sy cert)
```

---

## 2. Arrowhead 5.2 Certificate Profile Hierarchy

```
  profile-ca (Local Cloud CA)
  ══════════════════════════════════════════════════════════

  Root (lo) — self-signed, ephemeral
      │
      ├─── issues → Onboarding cert (OU=on)
      │             via HTTP :8087/bootstrap/onboarding-cert
      │             (no client cert required — step 1)
      │
      ├─── issues → Device cert (OU=de)
      │             via mTLS :8088/profile/device-cert
      │             (requires OU=on client cert — step 2)
      │
      └─── issues → System cert (OU=sy)
                    via mTLS :8088/profile/system-cert
                    (requires OU=de client cert — step 3)
                    ↓
                    Used as service identity at pki-rest-authz
                    (OU=sy enforced at TLS layer — step 4)

  Profile ordering is enforced — no step can be skipped.
  Presenting wrong profile cert → TLS rejection.
```

---

## 3. pki-consumer Certificate Lifecycle

```
  pki-consumer startup sequence
  ══════════════════════════════════════════════════════════

  [1] GET http://profile-ca:8087/ca/info
      → receive root CA certificate PEM

  [2] POST http://profile-ca:8087/bootstrap/onboarding-cert
      body: {"systemName":"pki-consumer"}
      → cert: {OU=on, CN=pki-consumer}
              └─ plain HTTP, no client cert needed

  [3] mTLS POST https://profile-ca:8088/profile/device-cert
      TLS client cert: OU=on
      → cert: {OU=de, CN=pki-consumer}

  [4] mTLS POST https://profile-ca:8088/profile/system-cert
      TLS client cert: OU=de
      → cert: {OU=sy, CN=pki-consumer}
              └─ identity cert for service access

  [loop] mTLS GET https://pki-rest-authz:9108/telemetry/latest
         TLS client cert: OU=sy, CN=pki-consumer
         PEP checks:
           (a) OU=sy?             → yes, proceed
           (b) AuthzForce Permit? → yes → proxy to data-provider-tls
                                  → no  → 403 Forbidden
```

---

## 4. Authorization Enforcement Path (all three transports)

```
  ConsumerAuth grants
          │
          ▼ (every SYNC_INTERVAL, mTLS)
  policy-sync ──PUT PolicySet──► AuthzForce (domain: arrowhead-exp8)
                                       │
              ┌────────────────────────┼────────────────────────┐
              │                        │                        │
              ▼                        ▼                        ▼
  topic-auth-xacml               kafka-authz             pki-rest-authz
  (AMQP broker plugin)           (Kafka SSE PEP)         (mTLS proxy PEP)
        │                              │                        │
        │ POST /pdp per               │ POST /pdp             │ POST /pdp
        │ broker operation            │ on connect            │ per request
        │                             │ + every 100 msgs      │ (+ OU=sy check)
        ▼                             ▼                        ▼
  consumer-1/2/3            analytics-consumer          pki-consumer
  (AMQP)                    (SSE stream)                (mTLS REST)
```

---

## 5. TLS Coverage Map

```
  ┌─────────────────────────────────────────────────────────────┐
  │  TLS Coverage in Experiment 8                               │
  │                                                             │
  │  Plain HTTP (internal/bootstrap only)                       │
  │  ─────────────────────────────────────────────────────────  │
  │  Core systems  :8080-8083    (Docker-internal only)         │
  │  AuthzForce    :8080         (Docker-internal only)         │
  │  policy-sync   :9105         (health/status/config)         │
  │  profile-ca    :8087         (bootstrap endpoint)           │
  │  kafka-authz   :9101         (health/status/auth-check)     │
  │  pki-rest-authz :9109        (health/status/auth-check)     │
  │                                                             │
  │  HTTPS / mTLS (TLS-secured)                                 │
  │  ─────────────────────────────────────────────────────────  │
  │  Core systems  :8490-8493    (mTLS, client cert required)   │
  │  profile-ca    :8088         (mTLS, profile cert required)  │
  │  pki-rest-authz :9108        (mTLS, OU=sy required)         │
  │  data-provider-tls :9094     (HTTPS server TLS)             │
  │                                                             │
  │  Broker TLS                                                 │
  │  ─────────────────────────────────────────────────────────  │
  │  RabbitMQ AMQPS :5671        (server TLS, AMQP credentials) │
  │  Kafka SSL      :9092        (server TLS, PKCS12 keystore)  │
  └─────────────────────────────────────────────────────────────┘
```

---

## 6. Dashboard Proxy Architecture

```
  Browser → nginx (port 3008)
  ════════════════════════════════════════════════════════

  /api/serviceregistry/   → http://serviceregistry:8080/
  /api/authentication/    → http://authentication:8081/
  /api/consumerauth/      → http://consumerauth:8082/
  /api/dynamicorch/       → http://dynamicorch:8083/
  /api/profile-ca/        → http://profile-ca:8087/       (HTTP bootstrap only)
  /api/authzforce/        → http://authzforce:8080/
  /api/policy-sync/       → http://policy-sync:9105/
  /api/topic-auth-xacml/  → http://topic-auth-xacml:9090/
  /api/kafka-authz/       → http://kafka-authz:9101/      (SSE: proxy_buffering off)
  /api/pki-rest-authz/    → http://pki-rest-authz:9109/   (HTTP health port, not mTLS)
  /api/pki-consumer/      → http://pki-consumer:9107/
  /api/analytics-consumer/→ http://analytics-consumer:9004/
  /api/robot-fleet/       → http://robot-fleet-tls:9003/
  /api/data-provider-tls/ → https://data-provider-tls:9094/  (TLS-verified via /certs/ca.crt)
  /api/rabbitmq/          → http://rabbitmq:15672/           (RabbitMQ management)

  Note: The browser cannot reach pki-rest-authz :9108 (mTLS) or profile-ca :8088 (mTLS).
  These are accessible only by services that possess appropriate profile certificates.
```

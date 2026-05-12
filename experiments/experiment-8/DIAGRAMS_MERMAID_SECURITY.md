# DIAGRAMS_MERMAID_SECURITY.md — Experiment 8

Security-focused Mermaid diagrams for experiment-8.
Covers: TLS trust model, profile enforcement, mTLS handshake, security gaps.

---

## 1. TLS Trust Model

```mermaid
graph TD
    ROOT["profile-ca Root (lo)\nself-signed ECDSA P-256\nephemeral — resets on restart"]

    ROOT -->|issues| ON["Onboarding Cert (OU=on)\nCN=systemName\nvia HTTP :8087"]
    ROOT -->|issues| DE["Device Cert (OU=de)\nCN=systemName\nvia mTLS :8088 + OU=on"]
    ROOT -->|issues| SY["System Cert (OU=sy)\nCN=systemName\nvia mTLS :8088 + OU=de"]
    ROOT -->|issues| INFRA["Infrastructure Certs\n(kafka, rabbitmq, core systems)\nvia cert-provisioner"]

    SY -->|identity at| PAZ["pki-rest-authz :9108\nOU=sy enforced at TLS"]
    DE -->|rejected at| PAZ
    ON -->|rejected at| PAZ

    ON -->|accepted at| PCATLS["profile-ca :8088\n(device cert issuance only)"]
    DE -->|accepted at| PCATLS2["profile-ca :8088\n(system cert issuance only)"]

    INFRA -->|kafka.crt/key| KFK["Kafka SSL :9092"]
    INFRA -->|rabbitmq.crt/key| RMQ["RabbitMQ AMQPS :5671"]
    INFRA -->|core *.crt/key| CORE["Core :8490-8493 (mTLS)"]
    INFRA -->|ca.crt| NGX["nginx proxy\n(verify data-provider-tls)"]
```

---

## 2. Profile Enforcement State Machine

```mermaid
stateDiagram-v2
    [*] --> NoIdentity : system starts

    NoIdentity --> HasOnboarding : POST /bootstrap/onboarding-cert\n(HTTP, no cert needed)
    HasOnboarding --> HasDevice : mTLS POST /profile/device-cert\n(present OU=on cert)
    HasDevice --> HasSystem : mTLS POST /profile/system-cert\n(present OU=de cert)
    HasSystem --> ServiceAccess : mTLS GET pki-rest-authz:9108\n(present OU=sy cert)

    ServiceAccess --> ServiceAccess : Permit → data received
    ServiceAccess --> Denied : Deny → 403 (no grant)
    Denied --> ServiceAccess : grant restored + SYNC_INTERVAL

    HasOnboarding --> Rejected1 : mTLS POST /profile/system-cert\n(skipping de — rejected)
    HasOnboarding --> Rejected2 : mTLS GET pki-rest-authz\n(wrong profile — TLS rejection)
    HasDevice --> Rejected3 : mTLS GET pki-rest-authz\n(wrong profile — TLS rejection)

    note right of Rejected1 : TLS rejection:\nwrong profile order
    note right of Rejected2 : TLS rejection:\nOU=on ≠ OU=sy
    note right of Rejected3 : TLS rejection:\nOU=de ≠ OU=sy
```

---

## 3. mTLS Handshake at pki-rest-authz

```mermaid
sequenceDiagram
    participant C as pki-consumer (OU=sy)
    participant P as pki-rest-authz :9108
    participant AZF as AuthzForce
    participant DPT as data-provider-tls

    Note over C,P: TLS 1.3 Handshake

    C->>P: ClientHello
    P-->>C: ServerHello + ServerCert (OU=sy, CN=pki-rest-authz)
    C->>C: verify server cert against root CA
    P->>P: request client cert
    C->>P: ClientCert (OU=sy, CN=pki-consumer)
    P->>P: verify client cert against root CA ✓
    P->>P: check cert.OU == "sy" ✓
    P->>P: extract identity = cert.CN = "pki-consumer"
    Note over C,P: TLS handshake complete

    P->>AZF: POST /pdp {subject="pki-consumer",\nresource="telemetry-rest", action="invoke"}

    alt Permit
        AZF-->>P: Decision: Permit
        P->>DPT: proxy GET /telemetry/latest (HTTPS, verified)
        DPT-->>P: 200 telemetry JSON
        P-->>C: 200 telemetry data
    else Deny
        AZF-->>P: Decision: Deny
        P-->>C: 403 Forbidden
    end
```

---

## 4. Profile Cert Issuance Sequence (mTLS enforcement at profile-ca)

```mermaid
sequenceDiagram
    participant C as any-service
    participant PCA_H as profile-ca HTTP :8087
    participant PCA_T as profile-ca mTLS :8088

    Note over C,PCA_T: Step 1: Bootstrap (no cert needed)
    C->>PCA_H: POST /bootstrap/onboarding-cert {systemName}
    PCA_H-->>C: cert {OU=on}

    Note over C,PCA_T: Step 2: Device cert (requires OU=on)
    C->>PCA_T: mTLS POST /profile/device-cert\nclient_cert: OU=on
    PCA_T->>PCA_T: verify client cert OU == "on" ✓
    PCA_T-->>C: cert {OU=de}

    Note over C,PCA_T: Attempt to skip to system cert
    C->>PCA_T: mTLS POST /profile/system-cert\nclient_cert: OU=on (WRONG)
    PCA_T->>PCA_T: verify client cert OU == "de" ✗
    PCA_T-->>C: TLS alert / 403 Forbidden

    Note over C,PCA_T: Step 3: System cert (requires OU=de)
    C->>PCA_T: mTLS POST /profile/system-cert\nclient_cert: OU=de
    PCA_T->>PCA_T: verify client cert OU == "de" ✓
    PCA_T-->>C: cert {OU=sy}

    Note over C,PCA_T: Identity cert ready for service access
```

---

## 5. Security Gap Status

```mermaid
quadrantChart
    title Security Gap Resolutions in Experiment 8
    x-axis Low Severity --> High Severity
    y-axis Not Resolved --> Resolved
    quadrant-1 Resolved high-severity
    quadrant-2 Resolved low-severity
    quadrant-3 Unresolved low-severity
    quadrant-4 Unresolved high-severity

    Gap-8.1 Profile hierarchy: [0.75, 0.92]
    Gap-8.2 Profile at PEP: [0.85, 0.88]
    Gap-8.3 Bootstrap plain HTTP: [0.3, 0.5]
    Gap-8.4 Core mTLS partial: [0.6, 0.45]
    Gap-8.5 Broker server-only TLS: [0.4, 0.3]
    Gap-8.6 XACML vs JWT: [0.5, 0.25]
    Gap-8.7 In-memory revocation: [0.45, 0.35]
```

---

## 6. Authorization Revocation Propagation

```mermaid
sequenceDiagram
    participant Op as Operator
    participant CA as ConsumerAuth :8492
    participant PS as policy-sync :9105
    participant AZF as AuthzForce
    participant C as pki-consumer

    Note over Op,C: Grant exists → access allowed

    C->>AZF: (via pki-rest-authz) check pki-consumer/telemetry-rest
    AZF-->>C: Permit

    Op->>CA: DELETE /authorization/revoke/{id}
    CA-->>Op: 200 revoked

    Note over PS,AZF: Up to SYNC_INTERVAL delay

    PS->>CA: GET /authorization/lookup (mTLS)
    CA-->>PS: [] (empty — grant removed)
    PS->>AZF: PUT PolicySet (no grant for pki-consumer)
    AZF-->>PS: 200 OK

    Note over C,AZF: Next request after sync

    C->>AZF: (via pki-rest-authz) check pki-consumer/telemetry-rest
    AZF-->>C: Deny
    Note over C: pki-consumer records deniedCount++

    Op->>CA: POST /authorization/grant (restore)
    CA-->>Op: 200 created

    Note over PS,AZF: Up to SYNC_INTERVAL delay

    PS->>AZF: PUT PolicySet (grant restored)

    C->>AZF: (via pki-rest-authz) check pki-consumer/telemetry-rest
    AZF-->>C: Permit
```

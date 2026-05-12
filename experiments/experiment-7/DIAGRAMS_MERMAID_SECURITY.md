# Experiment-7 — Security Architecture Diagrams

Mermaid versions of the diagrams in `DIAGRAMS.md`. The content is identical; only
the presentation format has changed from ASCII art to Mermaid.

---

## 1. TLS Trust Model

All certificates share a single self-signed ECDSA P-256 root CA (`ca:8086`). Two
provisioning paths exist: file-based (cert-provisioner init container, runs before the
stack), and runtime (Go services call the CA at startup to obtain their own cert).

```mermaid
graph TD
    CA["Root CA  ca:8086\nself-signed ECDSA P-256 · 10-year validity\nPOST /ca/certificate/issue\n(plain HTTP — bootstrap endpoint)"]

    subgraph file["File-based provisioning (cert-provisioner init container)"]
        CP["cert-provisioner\nwrites all certs to /certs volume\nexits 0 — runs before any service starts"]
    end

    subgraph runtime["Runtime provisioning (Go services at startup)"]
        RF["robot-fleet-tls"]
        CDT["consumer-direct-tls ×3"]
        CC["cert-consumer"]
    end

    CA -->|"issues"| CP
    CA -->|"issues"| RF
    CA -->|"issues"| CDT
    CA -->|"issues"| CC

    CP --> F1["ca.crt  (trust anchor for all services)"]
    CP --> F2["serviceregistry.crt / .key"]
    CP --> F3["authentication.crt / .key"]
    CP --> F4["consumerauth.crt / .key"]
    CP --> F5["dynamicorch.crt / .key"]
    CP --> F6["policy-sync.crt / .key"]
    CP --> F7["kafka.crt / .key + kafka-combined.pem"]
    CP --> F8["rabbitmq.crt / .key + rabbitmq-combined.pem"]
```

---

## 2. Service Communication — TLS Coverage

Solid arrows are TLS-protected connections. Dashed arrows are plain HTTP (Docker-internal
only). All host-accessible core-system ports are TLS; plain HTTP ports (8080–8083)
are restricted to Docker-internal traffic (healthchecks and bootstrap).

```mermaid
graph LR
    subgraph core["Core Systems"]
        SR["serviceregistry\n:8080 HTTP internal\n:8480 HTTPS/mTLS"]
        AUTH["authentication\n:8081 HTTP internal\n:8481 HTTPS/mTLS"]
        CA_AUTH["consumerauth\n:8082 HTTP internal\n:8482 HTTPS/mTLS"]
        DO["dynamicorch\n:8083 HTTP internal\n:8483 HTTPS/mTLS"]
    end

    CA_CERT["ca :8086\nplain HTTP only\n(trust anchor)"]
    AZ["authzforce :8186\nplain HTTP"]

    subgraph support["Support / PEPs"]
        PS["policy-sync"]
        TAX["topic-auth-xacml"]
        KA["kafka-authz"]
        CRA["cert-rest-authz\n:9098 mTLS proxy\n:9099 HTTP health"]
        DP["data-provider-tls :9094\nHTTPS"]
    end

    subgraph brokers["Brokers"]
        RMQ["RabbitMQ :5671\nAMQPS server-TLS"]
        KFK["Kafka :9092\nSSL server-TLS"]
    end

    subgraph clients["Experiment Services"]
        RF["robot-fleet-tls\n(runtime cert)"]
        CDT["consumer-direct-tls ×3\n(runtime cert)"]
        AC["analytics-consumer"]
        CC["cert-consumer\n(runtime cert)"]
    end

    %% mTLS
    RF  -->|"mTLS :8480"| SR
    RF  -->|"mTLS :8481"| AUTH
    CDT -->|"mTLS :8481"| AUTH
    CDT -->|"mTLS :8483"| DO
    DO  -->|"mTLS :8480"| SR
    DO  -->|"mTLS :8482"| CA_AUTH
    DO  -->|"mTLS :8481"| AUTH
    PS  -->|"mTLS :8482"| CA_AUTH
    CC  -->|"mTLS :9098\nclient cert CN = identity"| CRA

    %% HTTPS
    CRA -->|"HTTPS server-cert-verify"| DP

    %% plain HTTP (Docker-internal)
    PS  -.->|"HTTP"| AZ
    TAX -.->|"HTTP"| AZ
    KA  -.->|"HTTP"| AZ
    CRA -.->|"HTTP"| AZ
    RMQ -.->|"HTTP authz backend"| TAX

    %% AMQPS / Kafka SSL
    RF  -->|"AMQPS :5671"| RMQ
    RF  -->|"Kafka SSL :9092"| KFK
    CDT -->|"AMQPS :5671"| RMQ
    KA  -->|"Kafka SSL :9092"| KFK
    DP  -->|"Kafka SSL :9092"| KFK
    KA  -.->|"SSE stream"| AC

    %% CA bootstrap (plain HTTP — bootstrap constraint)
    RF  -.->|"HTTP cert issue"| CA_CERT
    CDT -.->|"HTTP cert issue"| CA_CERT
    CC  -.->|"HTTP cert issue"| CA_CERT
```

---

## 3. Certificate Provisioning Sequence

Two phases: cert-provisioner runs as an init container and provisions all file-based
certificates synchronously. After it exits 0, Docker starts the dependent services.
Go experiment services issue their own certificates at runtime.

```mermaid
sequenceDiagram
    participant CA as ca :8086
    participant CP as cert-provisioner
    participant INF as Kafka · RabbitMQ
    participant CORE as Core services · policy-sync
    participant GS as Go services (runtime)

    Note over CP: stack start — init container

    CP->>CA: GET /ca/info
    CA-->>CP: CA root cert (ca.crt)

    CP->>CA: POST /ca/certificate/issue  {systemName: "kafka"}
    CA-->>CP: kafka.crt + kafka.key

    CP->>CA: POST /ca/certificate/issue  {systemName: "rabbitmq"}
    CA-->>CP: rabbitmq.crt + rabbitmq.key

    CP->>CA: POST /ca/certificate/issue  {systemName: "serviceregistry"}
    CA-->>CP: serviceregistry.crt + .key

    CP->>CA: POST /ca/certificate/issue  {systemName: "authentication"}
    CA-->>CP: authentication.crt + .key

    CP->>CA: POST /ca/certificate/issue  {systemName: "consumerauth"}
    CA-->>CP: consumerauth.crt + .key

    CP->>CA: POST /ca/certificate/issue  {systemName: "dynamicorch"}
    CA-->>CP: dynamicorch.crt + .key

    CP->>CA: POST /ca/certificate/issue  {systemName: "policy-sync"}
    CA-->>CP: policy-sync.crt + .key

    Note over CP: exit 0 — all files written to /certs volume

    Note over INF,CORE: Docker starts dependents after cert-provisioner completes
    INF->>INF: start with TLS certs mounted from /certs
    CORE->>CORE: start with TLS certs mounted from /certs

    Note over GS: runtime cert issuance (robot-fleet-tls, consumer-direct-tls, cert-consumer)
    GS->>CA: POST /ca/certificate/issue  {systemName: "<own name>"}
    CA-->>GS: cert + key (held in memory)
    Note over GS: uses cert for all subsequent mTLS connections
```

---

## 4. mTLS Handshake — Core Service Path

Example: `consumer-direct-tls → dynamicorch:8483`. Both sides present certificates
issued by the same CA root; both sides verify the peer against that root.

```mermaid
sequenceDiagram
    participant CDT as consumer-direct-tls
    participant DO as dynamicorch :8483

    CDT->>DO: TLS ClientHello
    DO-->>CDT: TLS ServerHello + Certificate<br/>(CN=dynamicorch, SAN=dynamicorch)<br/>verified by CDT against /certs/ca.crt
    CDT->>DO: Certificate (CN=demo-consumer-1)<br/>verified by DO against /certs/ca.crt
    CDT->>DO: TLS Finished
    DO-->>CDT: TLS Finished

    Note over CDT,DO: Encrypted channel established — both identities verified

    CDT->>DO: HTTPS POST /orchestration/dynamic<br/>[Bearer token in Authorization header]
    DO-->>CDT: 200 OK  (orchestration result)
```

---

## 5. Gap Status Summary (G4)

G4 ("No mutual TLS") is fully closed for all core-system paths in experiment-7.

### Path status

| Path | Before G4 fix | After G4 fix |
|---|---|---|
| robot-fleet-tls → serviceregistry | plain HTTP | mTLS (runtime-issued cert) |
| robot-fleet-tls → authentication | plain HTTP | mTLS |
| consumer-direct-tls → authentication | plain HTTP | mTLS |
| consumer-direct-tls → dynamicorch | plain HTTP | mTLS |
| dynamicorch → serviceregistry | plain HTTP | mTLS (file cert) |
| dynamicorch → consumerauth | plain HTTP | mTLS |
| dynamicorch → authentication | plain HTTP | mTLS |
| policy-sync → consumerauth | plain HTTP | mTLS (file cert) |
| cert-consumer → cert-rest-authz | mTLS (unchanged) | mTLS (unchanged) |
| cert-rest-authz → data-provider-tls | HTTPS (unchanged) | HTTPS (unchanged) |

### Plain HTTP port exposure

| Service | Plain HTTP port | Host-exposed? | Used for |
|---|---|---|---|
| serviceregistry | 8080 | No (Docker-internal only) | Docker healthchecks |
| authentication | 8081 | No (Docker-internal only) | Docker healthchecks |
| consumerauth | 8082 | No (Docker-internal only) | Healthchecks + setup container bootstrap |
| dynamicorch | 8083 | No (Docker-internal only) | Docker healthchecks |
| ca | 8086 | Yes (required) | Bootstrap trust anchor — cannot self-authenticate |
| authzforce | 8080 (host: 8186) | Yes (required) | External Java service, HTTP-only |

The TLS ports 8480–8483 are the only host-accessible entry points for core systems.
`test-system.sh` section 14 verifies that ports 8080–8083 are connection-refused from the host.

### Items intentionally out of scope

| Item | Reason |
|---|---|
| CA plain HTTP | Bootstrap constraint — cannot use its own cert to authenticate itself |
| AuthzForce plain HTTP | External Java service — TLS configuration is outside this experiment's scope |
| Kafka / RabbitMQ client cert | Server-only TLS — brokers do not require client certificates (`KAFKA_SSL_CLIENT_AUTH: none`) |
| Docker internal healthchecks | Plain HTTP on Docker-internal network only — not reachable from host |

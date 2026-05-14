# DIAGRAMS.md — Experiment 9

Mermaid architectural diagrams for experiment-9.

Experiment 9 extends experiment-8 to the AIMS5.0 **UC3 "Lawn Mowing as a Service"** scenario:
three robot-fleet sites publish telemetry via Kafka (TLS) and AMQP (TLS); a **Portal & Cloud ML**
service aggregates the streams and exposes an HTTPS REST API; two **Service Partners** (SP1/SP2)
consume that API via the `pki-rest-authz` mTLS PEP.

---

## 1. Full System Component Diagram

```mermaid
graph TD
    subgraph Core["Arrowhead Core (mTLS :8490-8493)"]
        SR[ServiceRegistry :8490]
        AU[Authentication :8491]
        CA_SYS[ConsumerAuth :8492]
        DO[DynamicOrch :8493]
    end

    subgraph PKI["PKI Layer"]
        PCA["profile-ca\nHTTP :8087\nmTLS :8088"]
        CP[cert-provisioner\none-shot]
    end

    subgraph Policy["Policy Engine"]
        PS["policy-sync :9105\n(host :9205)"]
        AZF["AuthzForce\nXACML PDP/PAP\nport 8080\n(host :8296)"]
    end

    subgraph Brokers["Message Brokers (TLS)"]
        RMQ["RabbitMQ\nAMQPS :5671\n(mgmt host :15678)"]
        KFK["Kafka\nSSL :9092"]
    end

    subgraph AMQP_PEP["AMQP Policy Enforcement"]
        TAX["topic-auth-xacml\nAMQP auth plugin :9090"]
    end

    subgraph RobotSites["Robot Fleet Sites (UC3)"]
        RF1["robot-fleet-site-1\ncontrol host :9216"]
        RF2["robot-fleet-site-2\ncontrol host :9217"]
        RF3["robot-fleet-site-3\ncontrol host :9218"]
    end

    subgraph Portal["Portal & Cloud ML"]
        KAZ["kafka-authz\nKafka SSE PEP\nHTTP :9101 (host :9201)"]
        PCM["portal-cloud-ml\nHTTP :9207\nHTTPS REST :9294\n(host :9207)"]
    end

    subgraph RESTLayer["REST Access Layer"]
        PAZ["pki-rest-authz\nmTLS :9208\nHTTP :9209\n(host :9208/:9209)"]
    end

    subgraph SvcPartners["Service Partners (UC3)"]
        SP1["service-partner-1\nhealth host :9211"]
        SP2["service-partner-2\nhealth host :9212"]
    end

    DB["Dashboard\nnginx :80 (host :3009)"]

    PCA -->|cert issuance| CP
    CP -->|writes /certs| Brokers
    CP -->|writes /certs| Core

    CA_SYS -->|grants lookup mTLS| PS
    PS -->|PUT PolicySet| AZF

    AZF -->|XACML decisions| TAX
    AZF -->|XACML decisions| KAZ
    AZF -->|XACML decisions| PAZ

    TAX -->|auth plugin| RMQ

    RF1 & RF2 & RF3 -->|AMQP publish| RMQ
    RF1 & RF2 & RF3 -->|Kafka publish SSL| KFK

    KFK -->|consume SSL| KAZ
    KAZ -->|SSE stream| PCM

    PCA -->|PKI lifecycle on→de→sy| PCM
    PCM -->|HTTPS REST :9294| PAZ

    PCA -->|PKI lifecycle on→de→sy| PAZ
    PCA -->|PKI lifecycle on→de→sy| SP1
    PCA -->|PKI lifecycle on→de→sy| SP2

    SP1 & SP2 -->|mTLS GET /telemetry/latest| PAZ

    DB -->|/api proxies| Core
    DB -->|/api proxies| Policy
    DB -->|/api proxies| Portal
    DB -->|/api proxies| RESTLayer
    DB -->|/api proxies| RobotSites
```

---

## 2. UC3 Data Flow — Telemetry End-to-End

```mermaid
graph LR
    subgraph Site1["Site 1"]
        R1A[Robot A]
        R1B[Robot B]
    end
    subgraph Site2["Site 2"]
        R2A[Robot A]
        R2B[Robot B]
    end
    subgraph Site3["Site 3"]
        R3A[Robot A]
        R3B[Robot B]
    end

    subgraph Brokers["Kafka SSL :9092"]
        T["topic: arrowhead.telemetry"]
    end

    subgraph PEP1["kafka-authz (Kafka SSE PEP)"]
        KAZ["GET /stream/portal-cloud-ml\n?service=telemetry\n→ AuthzForce Permit"]
    end

    subgraph Aggregator["portal-cloud-ml"]
        SSE["SSE consumer\n(ConnectSSE)"]
        STORE["Store\n(latest payload\n+ msgCount)"]
        HTTPS["HTTPS REST :9294\n/telemetry/latest"]
    end

    subgraph PEP2["pki-rest-authz (mTLS REST PEP)"]
        mTLS["mTLS :9208\nOU=sy check\n→ AuthzForce Permit\n→ proxy → portal-cloud-ml:9294"]
    end

    subgraph Consumers["Service Partners"]
        SP1["service-partner-1\n(OU=sy, CN=service-partner-1)"]
        SP2["service-partner-2\n(OU=sy, CN=service-partner-2)"]
    end

    R1A & R1B & R2A & R2B & R3A & R3B -->|Kafka SSL| T
    T --> KAZ
    KAZ -->|SSE data lines| SSE
    SSE --> STORE
    STORE --> HTTPS
    HTTPS --> mTLS
    mTLS -->|mTLS| SP1
    mTLS -->|mTLS| SP2
```

---

## 3. Arrowhead 5.2 PKI Lifecycle (on → de → sy)

```mermaid
sequenceDiagram
    participant S as any-service\n(portal-cloud-ml or service-partner)
    participant PCA_H as profile-ca HTTP :8087
    participant PCA_T as profile-ca mTLS :8088

    Note over S,PCA_T: Step 1 — bootstrap (no cert needed)
    S->>PCA_H: GET /ca/info
    PCA_H-->>S: {commonName, certificate: root PEM}

    S->>PCA_H: POST /bootstrap/onboarding-cert {systemName}
    PCA_H-->>S: cert {OU=on, CN=systemName}

    Note over S,PCA_T: Step 2 — device cert (requires OU=on client cert)
    S->>PCA_T: mTLS POST /ca/device-cert\nclient_cert: OU=on
    PCA_T->>PCA_T: verify client OU == "on" ✓
    PCA_T-->>S: cert {OU=de, CN=systemName}

    Note over S,PCA_T: Step 3 — system cert (requires OU=de client cert)
    S->>PCA_T: mTLS POST /ca/system-cert\nclient_cert: OU=de
    PCA_T->>PCA_T: verify client OU == "de" ✓
    PCA_T-->>S: cert {OU=sy, CN=systemName}

    Note over S,PCA_T: Identity cert ready — service may now call pki-rest-authz
```

---

## 4. Service Partner Authorization Flow

```mermaid
sequenceDiagram
    participant SP as service-partner-N\n(OU=sy cert acquired)
    participant PAZ as pki-rest-authz :9208
    participant AZF as AuthzForce
    participant PCM as portal-cloud-ml :9294

    Note over SP,PCM: Every POLL_INTERVAL (default 5s)

    SP->>PAZ: mTLS GET /telemetry/latest\nX-Service-Name: telemetry-rest\nclient_cert: {OU=sy, CN=service-partner-N}

    PAZ->>PAZ: TLS handshake — verify client cert ✓
    PAZ->>PAZ: check cert.OU == "sy" ✓
    PAZ->>PAZ: extract identity = cert.CN

    PAZ->>AZF: POST /pdp\n{subject="service-partner-N",\nresource="portal-cloud-ml",\naction="telemetry-rest"}

    alt Permit
        AZF-->>PAZ: Decision: Permit
        PAZ->>PCM: proxy GET /telemetry/latest (HTTPS mTLS)
        PCM-->>PAZ: 200 telemetry JSON
        PAZ-->>SP: 200 telemetry data
        SP->>SP: stats.msgCount++
    else Deny
        AZF-->>PAZ: Decision: Deny
        PAZ-->>SP: 403 Forbidden
        SP->>SP: stats.deniedCount++
    end
```

---

## 5. portal-cloud-ml SSE Consumer Flow

```mermaid
sequenceDiagram
    participant PCM as portal-cloud-ml
    participant PCA_H as profile-ca :8087
    participant PCA_T as profile-ca :8088
    participant KAZ as kafka-authz :9101
    participant AZF as AuthzForce

    Note over PCM,AZF: Startup — acquire PKI system cert
    PCM->>PCA_H: GET /ca/info + POST /bootstrap/onboarding-cert
    PCA_H-->>PCM: cert {OU=on}
    PCM->>PCA_T: mTLS POST /ca/device-cert → cert {OU=de}
    PCM->>PCA_T: mTLS POST /ca/system-cert → cert {OU=sy}

    Note over PCM,AZF: Kafka SSE subscription loop
    loop ConnectSSE — reconnects on disconnect
        PCM->>KAZ: GET /stream/portal-cloud-ml?service=telemetry
        KAZ->>AZF: POST /pdp {subject="portal-cloud-ml",\nresource="robot-fleet-site-1",\naction="telemetry"}
        alt Permit
            AZF-->>KAZ: Permit
            KAZ-->>PCM: 200 text/event-stream
            loop Every robot telemetry message
                KAZ-->>PCM: data: {"site":"...", "val":...}
                PCM->>PCM: store.Record(payload)\nmsgCount++
            end
        else Deny
            AZF-->>KAZ: Deny
            KAZ-->>PCM: 403 Forbidden
            PCM->>PCM: deniedCount++
        end
    end
```

---

## 6. Policy Sync Sequence

```mermaid
sequenceDiagram
    participant CA as ConsumerAuth :8492
    participant PS as policy-sync :9105
    participant AZF as AuthzForce

    Note over PS,AZF: Startup — ensure domain arrowhead-exp9

    PS->>AZF: GET /domains?externalId=arrowhead-exp9
    alt domain missing
        PS->>AZF: POST /domains {externalId: "arrowhead-exp9"}
        AZF-->>PS: domain created
    else domain exists
        AZF-->>PS: domain info
    end

    Note over PS,AZF: UC3 grants seeded by setup container
    Note over PS,AZF: portal-cloud-ml → robot-fleet-site-1 / telemetry
    Note over PS,AZF: service-partner-1 → portal-cloud-ml / telemetry-rest
    Note over PS,AZF: service-partner-2 → portal-cloud-ml / telemetry-rest

    loop Every SYNC_INTERVAL (default 10s)
        PS->>CA: GET /authorization/lookup (mTLS)
        CA-->>PS: [{consumer, provider, service}, ...]
        PS->>PS: compile XACML PolicySet
        PS->>AZF: PUT /domains/{id}/pap/pdp.properties
        AZF-->>PS: 200 OK
    end
```

---

## 7. PKI Trust Hierarchy

```mermaid
graph TD
    ROOT["profile-ca Root (OU=lo)\nself-signed ECDSA P-256\nephemeral — resets on restart"]

    ROOT -->|HTTP :8087| ON["Onboarding Cert (OU=on)\nCN=systemName\nno client cert needed"]
    ROOT -->|mTLS :8088 + OU=on| DE["Device Cert (OU=de)\nCN=systemName"]
    ROOT -->|mTLS :8088 + OU=de| SY["System Cert (OU=sy)\nCN=systemName"]
    ROOT -->|cert-provisioner| INFRA["Infrastructure Certs\nkafka, rabbitmq, core,\npolicy-sync, pki-rest-authz"]

    SY -->|identity for| PAZ["pki-rest-authz :9208\nOU=sy enforced at TLS layer"]
    SY -->|used by| PCM["portal-cloud-ml :9294\nHTTPS server cert"]

    DE -->|rejected at| PAZ
    ON -->|rejected at| PAZ

    INFRA -->|kafka.crt/key| KFK["Kafka SSL :9092"]
    INFRA -->|rabbitmq.crt/key| RMQ["RabbitMQ AMQPS :5671"]
    INFRA -->|core *.crt/key| CORE["Core :8490-8493 mTLS"]
```

---

## 8. PKI Profile Enforcement State Machine

```mermaid
stateDiagram-v2
    [*] --> NoIdentity : service-partner or portal-cloud-ml starts

    NoIdentity --> HasOnboarding : POST /bootstrap/onboarding-cert\n(HTTP, no cert needed)
    HasOnboarding --> HasDevice : mTLS POST /ca/device-cert\n(present OU=on cert)
    HasDevice --> HasSystem : mTLS POST /ca/system-cert\n(present OU=de cert)
    HasSystem --> ServiceAccess : mTLS → pki-rest-authz :9208\n(present OU=sy cert)

    ServiceAccess --> ServiceAccess : Permit → telemetry received
    ServiceAccess --> Denied : Deny → 403 (no grant in AuthzForce)
    Denied --> ServiceAccess : grant restored + SYNC_INTERVAL passes

    HasOnboarding --> Rejected1 : mTLS POST /ca/system-cert\n(skipped de — TLS rejection)
    HasDevice --> Rejected2 : mTLS → pki-rest-authz\n(OU=de ≠ OU=sy — TLS rejection)
    HasOnboarding --> Rejected3 : mTLS → pki-rest-authz\n(OU=on ≠ OU=sy — TLS rejection)

    note right of Rejected1 : Profile ordering enforced:\ncannot skip de step
    note right of Rejected2 : PEP rejects non-sy certs\nat TLS handshake
```

---

## 9. Revocation and Re-grant Flow

```mermaid
sequenceDiagram
    participant Op as Operator
    participant CA as ConsumerAuth :8492
    participant PS as policy-sync :9105
    participant AZF as AuthzForce
    participant SP1 as service-partner-1

    Note over Op,SP1: Steady state — SP1 has access

    SP1->>AZF: (via pki-rest-authz) check service-partner-1 / telemetry-rest
    AZF-->>SP1: Permit ✓

    Op->>CA: DELETE /authorization/revoke/{id}
    CA-->>Op: 200 revoked

    Note over PS,AZF: Up to SYNC_INTERVAL (10s) propagation delay

    PS->>CA: GET /authorization/lookup
    CA-->>PS: [] (grant removed)
    PS->>AZF: PUT PolicySet (no rule for service-partner-1)
    AZF-->>PS: 200 OK

    SP1->>AZF: (via pki-rest-authz) check service-partner-1 / telemetry-rest
    AZF-->>SP1: Deny ✗
    Note over SP1: deniedCount++

    Op->>CA: POST /authorization/grant (restore)
    CA-->>Op: 201 created

    PS->>AZF: PUT PolicySet (grant restored)

    SP1->>AZF: (via pki-rest-authz) check service-partner-1 / telemetry-rest
    AZF-->>SP1: Permit ✓
    Note over SP1: msgCount resumes
```

---

## 10. TLS Coverage Map

```mermaid
graph LR
    subgraph Plain["Plain HTTP (internal / bootstrap only)"]
        H1["profile-ca :8087\n(onboarding bootstrap)"]
        H2["kafka-authz :9101\n(health + SSE stream)"]
        H3["pki-rest-authz :9209\n(health/stats only)"]
        H4["portal-cloud-ml :9207\n(health/stats only)"]
        H5["policy-sync :9105\n(health/status)"]
        H6["authzforce :8080\n(internal only)"]
        H7["core :8080-8083\n(internal health)"]
    end

    subgraph Secured["HTTPS / mTLS"]
        S1["profile-ca :8088\n(mTLS, OU=on or OU=de required)"]
        S2["pki-rest-authz :9208\n(mTLS, OU=sy required)"]
        S3["portal-cloud-ml :9294\n(HTTPS, server cert OU=sy)"]
        S4["core :8490-8493\n(mTLS, infra cert required)"]
    end

    subgraph BrokerTLS["Broker TLS"]
        B1["Kafka :9092\n(SSL server TLS)"]
        B2["RabbitMQ :5671\n(AMQPS server TLS)"]
    end
```

---

## 11. Dashboard Proxy Routing

```mermaid
graph LR
    Browser -->|HTTP :3009| NGX["nginx :80"]

    NGX -->|/api/serviceregistry/| SR["serviceregistry :8080"]
    NGX -->|/api/authentication/| AU["authentication :8081"]
    NGX -->|/api/consumerauth/| CA["consumerauth :8082"]
    NGX -->|/api/dynamicorch/| DO["dynamicorch :8083"]
    NGX -->|/api/authzforce/| AZF["authzforce :8080"]
    NGX -->|/api/policy-sync/| PS["policy-sync :9105"]
    NGX -->|/api/kafka-authz/\nSSE: proxy_buffering off| KAZ["kafka-authz :9101"]
    NGX -->|/api/pki-rest-authz/\nHTTP health port only| PAZ["pki-rest-authz :9209"]
    NGX -->|/api/portal-cloud-ml/| PCM["portal-cloud-ml :9207"]
    NGX -->|/api/service-partner-1/| SP1["service-partner-1 :9201"]
    NGX -->|/api/service-partner-2/| SP2["service-partner-2 :9202"]
    NGX -->|/api/robot-fleet-site-1/| RF1["robot-fleet-site-1 :9003"]
    NGX -->|/api/robot-fleet-site-2/| RF2["robot-fleet-site-2 :9003"]
    NGX -->|/api/robot-fleet-site-3/| RF3["robot-fleet-site-3 :9003"]
    NGX -->|/api/rabbitmq/| RMQ["rabbitmq :15672"]

    style PAZ fill:#fef3c7
    style PCM fill:#dcfce7
```

> **Note:** `pki-rest-authz :9208` (mTLS) and `portal-cloud-ml :9294` (HTTPS) are not proxied — the browser cannot present system certificates. Only service processes that completed the PKI lifecycle may connect to these endpoints.

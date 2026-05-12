# DIAGRAMS_MERMAID_BASICS.md — Experiment 8

Mermaid component and sequence diagrams for experiment-8.
For ASCII-art versions see DIAGRAMS.md.
For security-focused diagrams see DIAGRAMS_MERMAID_SECURITY.md.

---

## 1. System Component Diagram

```mermaid
graph TD
    subgraph Core["Arrowhead Core (mTLS :8490-8493)"]
        SR[ServiceRegistry :8490]
        AU[Authentication :8491]
        CA_SYS[ConsumerAuth :8492]
        DO[DynamicOrch :8493]
    end

    subgraph PKI["PKI Layer"]
        PCA[profile-ca\nHTTP :8087\nmTLS :8088]
        CP[cert-provisioner\none-shot]
    end

    subgraph Policy["Policy Engine"]
        PS[policy-sync :9105]
        AZF[AuthzForce\nXACML PDP/PAP\nport 8080]
    end

    subgraph Brokers["Message Brokers (TLS)"]
        RMQ[RabbitMQ\nAMQPS :5671]
        KFK[Kafka\nSSL :9092]
    end

    subgraph PEPs["Policy Enforcement Points"]
        TAX[topic-auth-xacml\nAMQP PEP :9090]
        KAZ[kafka-authz\nKafka SSE PEP :9101]
        PAZ[pki-rest-authz\nmTLS PEP :9108/:9109]
    end

    subgraph Experiment["Experiment Services"]
        RF[robot-fleet-tls\npublisher]
        C123[consumer-1/2/3\nAMQP]
        AC[analytics-consumer\nSSE]
        DPT[data-provider-tls\nHTTPS :9094]
        PKC[pki-consumer\nmTLS REST :9107]
    end

    DB[Dashboard\nnginx :3008]

    PCA -->|cert issuance| CP
    CP -->|writes /certs| Brokers
    CP -->|writes /certs| Core

    CA_SYS -->|grants/lookup mTLS| PS
    PS -->|PUT PolicySet| AZF

    AZF -->|XACML decisions| TAX
    AZF -->|XACML decisions| KAZ
    AZF -->|XACML decisions| PAZ

    TAX -->|auth plugin| RMQ
    KAZ -->|subscribe + SSE| KFK
    PAZ -->|proxy mTLS| DPT

    RF -->|AMQP publish| RMQ
    RF -->|Kafka publish| KFK
    KFK -->|consume| DPT

    RMQ -->|deliver| C123
    KAZ -->|SSE stream| AC
    PKC -->|mTLS GET| PAZ

    PCA -->|lifecycle certs| PKC
    PCA -->|lifecycle certs| DPT
    PCA -->|lifecycle certs| PS

    DB -->|/api proxies| Core
    DB -->|/api proxies| Policy
    DB -->|/api proxies| PEPs
    DB -->|/api proxies| Experiment
    DB -->|/api proxies| PCA
```

---

## 2. pki-consumer Certificate Lifecycle Sequence

```mermaid
sequenceDiagram
    participant C as pki-consumer
    participant PCA_H as profile-ca :8087 (HTTP)
    participant PCA_T as profile-ca :8088 (mTLS)
    participant PAZ as pki-rest-authz :9108
    participant DPT as data-provider-tls

    Note over C,DPT: Startup — PKI lifecycle (steps 1-4)

    C->>PCA_H: GET /ca/info
    PCA_H-->>C: {commonName, certificate: root PEM}

    C->>PCA_H: POST /bootstrap/onboarding-cert\n{systemName: "pki-consumer"}
    PCA_H-->>C: {certificate (OU=on), privateKey, profile:"on"}

    Note over C,PCA_T: Step 2 — mTLS with OU=on cert

    C->>PCA_T: mTLS POST /profile/device-cert\nclient_cert: OU=on
    PCA_T-->>C: {certificate (OU=de), privateKey, profile:"de"}

    Note over C,PCA_T: Step 3 — mTLS with OU=de cert

    C->>PCA_T: mTLS POST /profile/system-cert\nclient_cert: OU=de
    PCA_T-->>C: {certificate (OU=sy), privateKey, profile:"sy"}

    Note over C,DPT: Service loop — every POLL_INTERVAL

    loop Every 2s
        C->>PAZ: mTLS GET /telemetry/latest\nclient_cert: OU=sy, CN=pki-consumer
        PAZ->>PAZ: check OU=sy ✓\nCN="pki-consumer"
        PAZ->>PAZ: POST AuthzForce /pdp\nsubject=pki-consumer
        alt Permit
            PAZ->>DPT: proxy GET /telemetry/latest
            DPT-->>PAZ: telemetry JSON
            PAZ-->>C: 200 telemetry data
        else Deny
            PAZ-->>C: 403 Forbidden
        end
    end
```

---

## 3. Policy Sync Sequence

```mermaid
sequenceDiagram
    participant CA as ConsumerAuth :8492
    participant PS as policy-sync :9105
    participant AZF as AuthzForce

    Note over PS,AZF: Startup — create/ensure domain

    PS->>AZF: GET /domains?externalId=arrowhead-exp8
    alt domain exists
        AZF-->>PS: domain info
    else domain missing
        PS->>AZF: POST /domains {externalId: "arrowhead-exp8"}
        AZF-->>PS: domain created
    end

    loop Every SYNC_INTERVAL (default 10s)
        PS->>CA: GET /authorization/lookup (mTLS)
        CA-->>PS: [{id, consumerSystemName, providerSystemName, serviceDefinition}]
        PS->>PS: compile XACML PolicySet
        PS->>AZF: PUT /domains/{id}/pap/pdp.properties
        AZF-->>PS: 200 OK
        PS->>PS: update /status (version, grants, lastSyncedAt)
    end
```

---

## 4. AMQP Authorization Sequence

```mermaid
sequenceDiagram
    participant C as consumer-1/2/3
    participant RMQ as RabbitMQ :5671 (AMQPS)
    participant TAX as topic-auth-xacml
    participant AZF as AuthzForce

    C->>RMQ: AMQP CONNECT (credentials)
    RMQ->>TAX: auth_check (username, operation, resource)
    TAX->>AZF: POST /pdp {subject=username, resource=telemetry, action=consume}
    AZF-->>TAX: Permit / Deny
    TAX-->>RMQ: allow / deny
    alt Permit
        RMQ-->>C: connected
        RMQ-->>C: message delivery
    else Deny
        RMQ-->>C: access refused
    end
```

---

## 5. Dashboard Routing Diagram

```mermaid
graph LR
    B[Browser] -->|HTTP :3008| DB[nginx dashboard]

    DB -->|/api/serviceregistry/| SR[serviceregistry :8080]
    DB -->|/api/consumerauth/| CA[consumerauth :8082]
    DB -->|/api/profile-ca/| PCA[profile-ca :8087]
    DB -->|/api/authzforce/| AZF[authzforce :8080]
    DB -->|/api/policy-sync/| PS[policy-sync :9105]
    DB -->|/api/kafka-authz/\nSSE: proxy_buffering off| KAZ[kafka-authz :9101]
    DB -->|/api/pki-rest-authz/\nHTTP health port only| PAZ[pki-rest-authz :9109]
    DB -->|/api/pki-consumer/| PKC[pki-consumer :9107]
    DB -->|/api/analytics-consumer/| AC[analytics-consumer :9004]
    DB -->|/api/robot-fleet/| RF[robot-fleet-tls :9003]
    DB -->|/api/data-provider-tls/\nhttps + proxy_ssl_verify on| DPT[data-provider-tls :9094]
    DB -->|/api/rabbitmq/| RMQ[rabbitmq :15672]

    style PAZ fill:#fef3c7
    style DPT fill:#dcfce7
    note1[/"Note: pki-rest-authz :9108 (mTLS)\nnot proxied — browser cannot\npresent system certs"/]
```

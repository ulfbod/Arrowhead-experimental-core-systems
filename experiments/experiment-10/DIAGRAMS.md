# DIAGRAMS.md — Experiment 10

Mermaid architectural diagrams for experiment-10.

Experiment-10 extends experiment-9 with a **clean PAP/PIP/PDP access-control architecture**:
the `policy-sync` + ConsumerAuth source-of-truth pattern is replaced by a dedicated
**PAP** (Policy Administration Point) that pushes XACML policies to AuthzForce immediately
on every Create/Delete, and a **PIP** (Policy Information Point) for subject attribute resolution.

---

## 1. Full System Component Diagram

```mermaid
graph TD
    subgraph Core["Arrowhead Core (mTLS :8490-8493)"]
        SR[ServiceRegistry :8490]
        AU[Authentication :8491]
        CA_SYS[ConsumerAuth :8492\northestration only]
        DO[DynamicOrch :8493]
    end

    subgraph PKI["PKI Layer"]
        PCA["profile-ca\nHTTP :8087\nmTLS :8088"]
        CP[cert-provisioner\none-shot]
    end

    subgraph PolicyEngine["Policy Engine (PAP/PIP/PDP)"]
        PAP["PAP :9305\nPolicy Administration Point\nsource of truth"]
        PIP["PIP :9306\nPolicy Information Point\nsubject attributes"]
        AZF["AuthzForce :8080\nXACML PDP\narrowhead-exp10"]
    end

    subgraph Enforcement["PEPs — Enforcement"]
        TAX["topic-auth-xacml\nAMQP XACML PEP"]
        KA["kafka-authz :9101\nKafka XACML PEP"]
        PRA["pki-rest-authz\nmTLS :9208 / HTTP :9209\nOU=sy + XACML PEP"]
    end

    subgraph Messaging["Messaging Infrastructure"]
        RMQ[RabbitMQ AMQPS :5671]
        KF[Kafka SSL :9092]
    end

    subgraph UC3["UC3 Data Plane"]
        S1["robot-fleet-site-1 :9003"]
        S2["robot-fleet-site-2 :9003"]
        S3["robot-fleet-site-3 :9003"]
        PCM["portal-cloud-ml :9207\nKafka SSE → REST"]
        SP1["service-partner-1 :9201"]
        SP2["service-partner-2 :9202"]
    end

    ADMIN["Admin Dashboard :3010/admin.html"]

    PCA -->|issue infra certs| CP
    CP -->|write to /certs| Core

    PAP -->|BuildPolicy+SetPolicy| AZF
    ADMIN -->|POST/DELETE /policies| PAP
    ADMIN -->|POST /subjects| PIP

    TAX -->|XACML Decide| AZF
    KA  -->|XACML Decide| AZF
    PRA -->|verify OU=sy + XACML Decide| AZF
    PRA -->|verify cert chain| PCA

    S1 -->|AMQP TLS| RMQ
    S2 -->|AMQP TLS| RMQ
    S3 -->|AMQP TLS| RMQ
    S1 -->|Kafka SSL| KF
    S2 -->|Kafka SSL| KF
    S3 -->|Kafka SSL| KF

    RMQ -->|authz check| TAX
    KF  -->|SSE stream| KA
    KA  -->|SSE forward| PCM
    PCM -->|HTTPS REST| PRA
    PRA -->|proxy| SP1
    PRA -->|proxy| SP2

    DO -->|lookup| SR
    DO -->|lookup| CA_SYS
```

---

## 2. PAP/PIP/PDP Interaction Flow

```mermaid
sequenceDiagram
    participant Admin
    participant PAP
    participant AZF as AuthzForce PDP
    participant PEP as pki-rest-authz PEP
    participant SP1 as service-partner-1

    Admin->>PAP: POST /policies {subject,resource,action,effect}
    PAP->>AZF: BuildPolicy() → SetPolicy() (XACML PolicySet)
    AZF-->>PAP: 200 OK
    PAP-->>Admin: 201 Created {id,...}

    SP1->>PEP: HTTPS GET /telemetry/latest (OU=sy client cert)
    PEP->>PEP: verify OU=sy in client cert CN
    PEP->>AZF: XACML Decide {subject=service-partner-1, resource=telemetry-rest}
    AZF-->>PEP: Permit
    PEP->>SP1: 200 OK (proxied to portal-cloud-ml)

    Admin->>PAP: DELETE /policies/{id}
    PAP->>AZF: BuildPolicy() → SetPolicy() (updated PolicySet, SP1 removed)
    AZF-->>PAP: 200 OK
    PAP-->>Admin: 204 No Content

    SP1->>PEP: HTTPS GET /telemetry/latest (OU=sy client cert)
    PEP->>AZF: XACML Decide {subject=service-partner-1, resource=telemetry-rest}
    AZF-->>PEP: Deny
    PEP-->>SP1: 403 Forbidden
```

---

## 3. UC3 Data Flow

```mermaid
sequenceDiagram
    participant RF as Robot Fleet Site N
    participant KF as Kafka SSL
    participant KA as kafka-authz
    participant PCM as portal-cloud-ml
    participant PRA as pki-rest-authz
    participant SP as Service Partner

    RF->>KF: publish arrowhead.telemetry (SSL, OU=sy cert)
    KA->>AZF: XACML Decide {subject=portal-cloud-ml, resource=telemetry}
    AZF-->>KA: Permit
    KA->>PCM: SSE forward
    PCM->>PCM: aggregate telemetry

    SP->>PRA: GET /telemetry/latest (mTLS, OU=sy cert)
    Note over PRA: verify CN == "service-partner-N" AND OU=sy
    PRA->>AZF: XACML Decide {subject=service-partner-N, resource=telemetry-rest}
    AZF-->>PRA: Permit
    PRA->>PCM: GET /telemetry/latest (forwarded)
    PCM-->>PRA: 200 {payload}
    PRA-->>SP: 200 {payload}
```

---

## 4. PKI Certificate Hierarchy

```mermaid
graph TD
    LO["lo — Local Cloud CA\nself-signed root\nprofile-ca :8087/:8088"]
    ON["on — Onboarding cert\nPOST /bootstrap/onboarding-cert\n(plain HTTP, no auth)"]
    DE["de — Device cert\nmTLS POST /ca/device-cert\nrequires OU=on client cert"]
    SY["sy — System cert\nmTLS POST /ca/system-cert\nrequires OU=de client cert"]

    LO --> ON
    ON --> DE
    DE --> SY

    SY -->|"used as: server cert at pki-rest-authz :9208"| PRA["pki-rest-authz\nOU=sy enforced at TLS"]
    SY -->|"used as: client cert by service partners"| SP["service-partner-1/2\nidentity in XACML request"]
    SY -->|"used as: server cert at portal-cloud-ml :9294"| PCM["portal-cloud-ml\nHTTPS REST backend"]
```

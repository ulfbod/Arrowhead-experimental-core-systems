# Experiment 3 — Arrowhead Core System Interactions

This file documents how experiment-3 relates to and interacts with the
Arrowhead 5 core systems.

---

## AHC System Landscape

Experiment-3 uses **one** of the six core systems. The others are
intentionally absent — this experiment isolates the ConsumerAuthorization
policy mechanism without involving service discovery, identity, or
orchestration.

```mermaid
graph LR
    subgraph ahc["Arrowhead Core Systems"]
        SR["ServiceRegistry\n:8080"]
        AUTH["Authentication\n:8081"]
        CA["ConsumerAuthorization\n:8082"]
        DO["DynamicOrchestration\n:8083"]
        SSO["SimpleStoreOrchestration\n:8084"]
        FSO["FlexibleStoreOrchestration\n:8085"]
    end

    subgraph exp3["Experiment-3"]
        TAS["topic-auth-sync"]
        DB["Dashboard"]
        Setup["setup (one-shot)"]
    end

    Setup -->|"POST /authorization/grant"| CA
    TAS   -->|"GET /authorization/lookup\nevery 10 s"| CA
    DB    -->|"GET /authorization/lookup\nPOST /authorization/grant\nDELETE /authorization/revoke/{id}"| CA

    SR  ~~~|"not used"| exp3
    AUTH~~~|"not used"| exp3
    DO  ~~~|"not used"| exp3
    SSO ~~~|"not used"| exp3
    FSO ~~~|"not used"| exp3

    style CA   fill:#d1fae5,stroke:#6ee7b7
    style SR   fill:#f3f4f6,stroke:#d1d5db,color:#9ca3af
    style AUTH fill:#f3f4f6,stroke:#d1d5db,color:#9ca3af
    style DO   fill:#f3f4f6,stroke:#d1d5db,color:#9ca3af
    style SSO  fill:#f3f4f6,stroke:#d1d5db,color:#9ca3af
    style FSO  fill:#f3f4f6,stroke:#d1d5db,color:#9ca3af
```

| Core system | Used | Reason |
|---|---|---|
| **ConsumerAuthorization** | ✔ | Authoritative source of consumer-to-service grants; polled by `topic-auth-sync` |
| ServiceRegistry | — | Services locate each other via Docker DNS and environment variables, not AHC discovery |
| Authentication | — | No inter-system identity tokens required in this experiment |
| DynamicOrchestration | — | Service binding is static (AMQP routing key); no runtime endpoint negotiation needed |
| SimpleStoreOrchestration | — | (see above) |
| FlexibleStoreOrchestration | — | (see above) |

---

## ConsumerAuthorization: API Surface Used

```mermaid
graph LR
    subgraph CA["ConsumerAuthorization :8082"]
        G["POST /authorization/grant"]
        R["DELETE /authorization/revoke/{id}"]
        L["GET /authorization/lookup"]
        V["POST /authorization/verify\n(defined but not called\nin experiment-3)"]
        T["POST /authorization/token/generate\n(defined but not called\nin experiment-3)"]
        H["GET /health"]
    end

    Setup["setup (one-shot)"]
    TAS["topic-auth-sync"]
    DB["Dashboard"]

    Setup -->|seed initial grants| G
    DB    -->|operator adds grant| G
    DB    -->|operator revokes grant| R
    DB    -->|display grant list| L
    TAS   -->|reconcile RabbitMQ\npermissions| L
    DB    -->|liveness probe| H

    style V fill:#f9fafb,stroke:#e5e7eb,color:#9ca3af
    style T fill:#f9fafb,stroke:#e5e7eb,color:#9ca3af
```

---

## Data Model: AuthRule

The only data structure exchanged between experiment-3 services and
ConsumerAuthorization.

```mermaid
classDiagram
    class AuthRule {
        +int64  id
        +string consumerSystemName
        +string providerSystemName
        +string serviceDefinition
    }

    class LookupResponse {
        +AuthRule[] rules
        +int        count
    }

    class GrantRequest {
        +string consumerSystemName
        +string providerSystemName
        +string serviceDefinition
    }

    LookupResponse "1" --> "*" AuthRule : contains
    GrantRequest ..> AuthRule : creates
```

---

## Sequence: Seeding Initial Grants (startup)

The `setup` one-shot container calls ConsumerAuthorization once at startup to
create the three default grants. It accepts HTTP 409 (already exists) as a
success condition so the stack can be restarted without re-seeding.

```mermaid
sequenceDiagram
    participant Setup as setup (one-shot)
    participant CA as ConsumerAuthorization :8082

    Note over CA: in-memory store is empty on first start

    Setup->>CA: POST /authorization/grant<br/>{"consumerSystemName":"demo-consumer-1","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}
    CA-->>Setup: 201 Created  {"id":1, ...}

    Setup->>CA: POST /authorization/grant<br/>{"consumerSystemName":"demo-consumer-2", ...}
    CA-->>Setup: 201 Created  {"id":2, ...}

    Setup->>CA: POST /authorization/grant<br/>{"consumerSystemName":"demo-consumer-3", ...}
    CA-->>Setup: 201 Created  {"id":3, ...}

    Note over Setup: exits 0 → topic-auth-sync dependency satisfied

    Note over CA: on restart of the stack:<br/>POST returns 409 "authorization rule already exists"<br/>setup treats 409 as success and still exits 0
```

---

## Sequence: topic-auth-sync Reconciliation Loop

Every 10 seconds `topic-auth-sync` queries ConsumerAuthorization and
translates the grant list into RabbitMQ users and topic permissions. This is
the central bridge between the AHC policy layer and the AMQP broker.

```mermaid
sequenceDiagram
    participant CA as ConsumerAuthorization :8082
    participant TAS as topic-auth-sync
    participant RMQ as RabbitMQ (Management API)

    loop every 10 s
        TAS->>CA: GET /authorization/lookup
        CA-->>TAS: {"rules":[{id,consumerSystemName,providerSystemName,serviceDefinition},…],"count":N}

        Note over TAS: derive topic read pattern per consumer<br/>"telemetry" → ^telemetry\.<br/>"telemetry"+"sensors" → ^(sensors|telemetry)\.

        TAS->>RMQ: PUT /api/users/robot-fleet + permissions (write: ^telemetry\.)
        loop for each consumer in rules
            TAS->>RMQ: PUT /api/users/{consumerSystemName}
            TAS->>RMQ: PUT /api/permissions/%2F/{consumerSystemName}
            TAS->>RMQ: PUT /api/topic-permissions/%2F/{consumerSystemName}  (read: {pattern})
        end
        loop for each arrowhead-managed user NOT in rules
            TAS->>RMQ: DELETE /api/users/{staleUser}
        end

        Note over TAS: /health returns 200 once first sync succeeds
    end
```

---

## Sequence: Grant Lifecycle via Dashboard

The operator interacts with ConsumerAuthorization through the experiment-3
dashboard. Changes are picked up by `topic-auth-sync` within one sync cycle
(≤ 10 s).

```mermaid
sequenceDiagram
    actor Operator
    participant DB as Dashboard :3003
    participant CA as ConsumerAuthorization :8082
    participant TAS as topic-auth-sync

    Operator->>DB: open Grants & Sync tab

    DB->>CA: GET /authorization/lookup
    CA-->>DB: {"rules":[…],"count":3}
    Note over DB: renders grant table +<br/>derived topic patterns

    Operator->>DB: fill in consumer name + service, click Add
    DB->>CA: POST /authorization/grant<br/>{"consumerSystemName":"…","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}
    CA-->>DB: 201 Created  {"id":4, …}
    DB->>CA: GET /authorization/lookup  (refresh)
    CA-->>DB: {"rules":[…],"count":4}

    Note over TAS: within 10 s: next sync cycle creates new RabbitMQ user

    Operator->>DB: click Revoke on a row
    DB->>CA: DELETE /authorization/revoke/4
    CA-->>DB: 200 OK
    DB->>CA: GET /authorization/lookup  (refresh)
    CA-->>DB: {"rules":[…],"count":3}

    Note over TAS: within 10 s: next sync cycle deletes stale RabbitMQ user
```

---

## What a Full AHC Integration Would Add

The following interactions are **not implemented** in experiment-3 but would be
present in a fully AHC-integrated deployment:

```mermaid
graph TD
    subgraph "Not implemented in experiment-3"
        SR["ServiceRegistry :8080"]
        AUTH["Authentication :8081"]
        DO["DynamicOrchestration :8083"]
    end

    RF_reg["robot-fleet\nregister service"]
    C_orch["consumer\norchestrate"]
    TAS_verify["topic-auth-sync\nverify before sync"]

    RF_reg -->|"POST /serviceregistry/register\n{serviceDefinition:telemetry, address:rabbitmq, port:5672}"| SR
    C_orch  -->|"POST /orchestration/dynamic\n{consumer, serviceDefinition:telemetry}"| DO
    DO      -->|"POST /serviceregistry/query"| SR
    DO      -->|"POST /authorization/verify\n(optional filter)"| AUTH
    TAS_verify -->|"POST /authorization/verify\nbefore provisioning"| AUTH

    style SR   fill:#fef9c3,stroke:#fde047
    style AUTH fill:#fef9c3,stroke:#fde047
    style DO   fill:#fef9c3,stroke:#fde047
```

| Missing interaction | What it would provide |
|---|---|
| robot-fleet → ServiceRegistry | Consumers could discover the AMQP endpoint dynamically instead of using a hardcoded env var |
| consumer → DynamicOrchestration | Runtime binding: orchestrator looks up the AMQP endpoint and verifies the consumer is authorized |
| topic-auth-sync → Authentication | Tokens on calls to ConsumerAuthorization, preventing unauthorized policy reads |

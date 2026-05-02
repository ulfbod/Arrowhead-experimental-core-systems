# Experiment 4 — Diagrams

## Component Diagram

Shows all services, their roles, and how they connect.

```mermaid
graph TD
    subgraph core["Arrowhead Core"]
        SR["ServiceRegistry :8080\n─────────────\nstores service\nregistrations"]
        AUTH["Authentication :8081\n─────────────\nissues and verifies\nBearer tokens"]
        CA_auth["ConsumerAuth :8082\n─────────────\nstores authorization\ngrants in memory"]
        DO["DynamicOrch :8083\n─────────────\nverify identity\nquery SR + CA\nreturn endpoint"]
        CA_cert["CertAuth :8086\n─────────────\nissues X.509 certs\n(Phase 5 — not yet wired)"]
    end

    subgraph support["Support Services"]
        TAS["topic-auth-sync :9090\n─────────────\npolls ConsumerAuth\nreconciles RabbitMQ\nusers & permissions"]
        RMQ["RabbitMQ :5672 / :15674\n─────────────\ntopic exchange: arrowhead\nauth_backend_topic plugin"]
    end

    subgraph experiment["Experiment Services"]
        RF["robot-fleet :9003\n─────────────\npublishes synthetic\ntelemetry to AMQP\nregisters in SR"]
        C1["consumer-1"]
        C2["consumer-2"]
        C3["consumer-3"]
    end

    DB["Dashboard :3004\n─────────────\nhealth · grants · live data"]

    RF  -->|"POST /serviceregistry/register\nat startup"| SR
    RF  -->|"POST /authentication/identity/login\nat startup"| AUTH

    TAS -->|"GET /authorization/lookup\nevery 10 s  [Bearer token]"| CA_auth
    TAS -->|"Management API\nPUT users + permissions\nDELETE stale users"| RMQ
    TAS -->|"POST /authentication/identity/login\nat startup"| AUTH

    C1  -->|"POST /authentication/identity/login"| AUTH
    C1  -->|"POST /orchestration/dynamic\n[Bearer token]"| DO
    C1  -->|"POST /authorization/token/generate"| CA_auth
    C1  -->|"AMQP subscribe\nbinding key from SR metadata"| RMQ

    C2  -->|"same AHC flow"| DO
    C3  -->|"same AHC flow"| DO

    DO  -->|"GET /authentication/identity/verify"| AUTH
    DO  -->|"POST /serviceregistry/query"| SR
    DO  -->|"POST /authorization/verify"| CA_auth

    RF  -->|"AMQP publish\nrouting key: telemetry.{robotId}"| RMQ

    DB -.->|health + grants| CA_auth
    DB -.->|management API| RMQ
    DB -.->|health| TAS
    DB -.->|health| SR
    DB -.->|health| AUTH
    DB -.->|health| DO
    DB -.->|health| CA_cert
    DB -.->|health + stats| RF
    DB -.->|health + stats| C1
    DB -.->|health + stats| C2
    DB -.->|health + stats| C3

    style CA_cert fill:#f9fafb,stroke:#e5e7eb,color:#9ca3af
```

---

## Sequence Diagram 1 — Startup

`setup` seeds grants, `topic-auth-sync` authenticates and runs its first
reconciliation, `robot-fleet` authenticates and registers in ServiceRegistry,
then consumers start.

```mermaid
sequenceDiagram
    actor Operator
    participant Setup
    participant SR as ServiceRegistry
    participant Auth as Authentication
    participant CA as ConsumerAuth
    participant DO as DynamicOrch
    participant TAS as topic-auth-sync
    participant RMQ as RabbitMQ
    participant RF as robot-fleet
    participant C as consumer-1/2/3

    Operator->>Setup: docker compose up

    Note over SR,DO: Core systems start in parallel

    Setup->>CA: POST /authorization/grant ×3
    CA-->>Setup: 201 Created

    TAS->>Auth: POST /authentication/identity/login {topic-auth-sync}
    Auth-->>TAS: {token: "…"}

    loop first sync cycle
        TAS->>CA: GET /authorization/lookup  [Bearer token]
        CA-->>TAS: [{consumer-1,telemetry}, {consumer-2,telemetry}, {consumer-3,telemetry}]
        TAS->>RMQ: PUT /api/users/robot-fleet + topic permissions
        TAS->>RMQ: PUT /api/users/demo-consumer-{1,2,3} + topic permissions
        TAS-->>TAS: ready=true → /health returns 200
    end

    RF->>Auth: POST /authentication/identity/login {robot-fleet}
    Auth-->>RF: {token: "…"}
    RF->>SR: POST /serviceregistry/register {telemetry, rabbitmq:5672, arrowhead exchange}
    SR-->>RF: 201 Created
    RF->>RMQ: AMQP connect (robot-fleet : fleet-secret)

    C->>Auth: POST /authentication/identity/login {demo-consumer-N}
    Auth-->>C: {token: "…"}
    C->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>Auth: GET /authentication/identity/verify [token] → verified demo-consumer-N
    DO->>SR: POST /serviceregistry/query {telemetry}
    SR-->>DO: [{provider: rabbitmq:5672, serviceUri: arrowhead, …}]
    DO->>CA: POST /authorization/verify {consumer-N, robot-fleet, telemetry}
    CA-->>DO: {authorized: true}
    DO-->>C: {provider: {address: rabbitmq, port: 5672}, service: {serviceUri: arrowhead, metadata: {routingKeyPattern: telemetry.*}}}
    C->>CA: POST /authorization/token/generate
    CA-->>C: {token: "…"}
    C->>RMQ: AMQP connect (demo-consumer-N : consumer-secret)
    C->>RMQ: QueueBind(telemetry.#) — topic check: ^telemetry\. ✔
```

---

## Sequence Diagram 2 — Normal message flow

Once connected, robot-fleet publishes telemetry and RabbitMQ fans it out to
every consumer whose queue is bound to a matching routing key.

```mermaid
sequenceDiagram
    participant RF as robot-fleet
    participant RMQ as RabbitMQ<br/>(exchange: arrowhead)
    participant C1 as consumer-1
    participant C2 as consumer-2
    participant C3 as consumer-3

    loop every ~100 ms per robot
        RF->>RMQ: Publish(key=telemetry.robot-1, payload=…)
        Note over RMQ: topic check write: ^telemetry\. matches ✔
        RMQ->>C1: Deliver → demo-consumer-1-queue
        RMQ->>C2: Deliver → demo-consumer-2-queue
        RMQ->>C3: Deliver → demo-consumer-3-queue
    end
```

---

## Sequence Diagram 3 — Revoke and re-grant: dual-layer enforcement

Revoking a grant is effective immediately at the orchestration layer (DO returns
empty) and within ≤ 10 s at the broker layer (topic-auth-sync deletes the user).
Re-granting restores access via the same two-step path.

```mermaid
sequenceDiagram
    actor Operator
    participant CA as ConsumerAuth
    participant DO as DynamicOrch
    participant TAS as topic-auth-sync
    participant RMQ as RabbitMQ
    participant C2 as consumer-2

    Note over C2,RMQ: consumer-2 connected and receiving messages

    Operator->>CA: DELETE /authorization/revoke/{id}
    CA-->>Operator: 200 OK

    Note over C2: connection drops or next retry loop (≤ 5 s)
    C2->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>CA: POST /authorization/verify {consumer-2, robot-fleet, telemetry}
    CA-->>DO: {authorized: false}
    DO-->>C2: {response: []}  (empty — no authorized providers)
    Note over C2: "no authorized providers" → wait 5 s, retry

    loop next sync cycle (≤ 10 s)
        TAS->>CA: GET /authorization/lookup
        Note over TAS: demo-consumer-2 absent → stale user
        TAS->>RMQ: DELETE /api/users/demo-consumer-2
    end

    Operator->>CA: POST /authorization/grant {consumer-2, robot-fleet, telemetry}
    CA-->>Operator: 201 Created

    C2->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>CA: POST /authorization/verify {consumer-2, robot-fleet, telemetry}
    CA-->>DO: {authorized: true}
    DO-->>C2: {provider: rabbitmq:5672, …}
    C2->>CA: POST /authorization/token/generate
    C2->>RMQ: AMQP connect → authenticated ✔
    C2->>RMQ: QueueBind(telemetry.#) — topic check: ^telemetry\. ✔
    Note over C2,RMQ: consumer-2 resumes receiving messages
```

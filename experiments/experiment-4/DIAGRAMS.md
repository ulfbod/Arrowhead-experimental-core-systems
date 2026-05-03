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
        TAH["topic-auth-http :9090\n─────────────\nserves RabbitMQ HTTP\nauthz backend API\n+ background reconciliation\n(user lifecycle)"]
        RMQ["RabbitMQ :5672 / :15674\n─────────────\ntopic exchange: arrowhead\nauthn=internal\nauthz=HTTP backend"]
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

    TAH -->|"GET /authorization/lookup\nlive per-request + background sync\n[Bearer token]"| CA_auth
    TAH -->|"Management API\nPUT users (background sync)\nDELETE stale users"| RMQ
    TAH -->|"POST /authentication/identity/login\nat startup"| AUTH

    RMQ -->|"POST /auth/vhost\non every new connection"| TAH
    RMQ -->|"POST /auth/resource\non exchange/queue ops"| TAH
    RMQ -->|"POST /auth/topic\non every publish + bind"| TAH

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
    DB -.->|health| TAH
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

`setup` seeds grants, `topic-auth-http` authenticates and runs its first
reconciliation (creating RabbitMQ users), then `robot-fleet` and consumers start.
RabbitMQ calls `topic-auth-http` for every vhost and topic authz check.

```mermaid
sequenceDiagram
    actor Operator
    participant Setup
    participant SR as ServiceRegistry
    participant Auth as Authentication
    participant CA as ConsumerAuth
    participant DO as DynamicOrch
    participant TAH as topic-auth-http
    participant RMQ as RabbitMQ
    participant RF as robot-fleet
    participant C as consumer-1/2/3

    Operator->>Setup: docker compose up

    Note over SR,DO: Core systems start in parallel

    Setup->>CA: POST /authorization/grant ×3
    CA-->>Setup: 201 Created

    TAH->>Auth: POST /authentication/identity/login {topic-auth-http}
    Auth-->>TAH: {token: "…"}

    loop first reconciliation (user lifecycle sync)
        TAH->>CA: GET /authorization/lookup  [Bearer token]
        CA-->>TAH: [{consumer-1,telemetry}, {consumer-2,telemetry}, {consumer-3,telemetry}]
        TAH->>RMQ: PUT /api/users/robot-fleet + permissions (management API)
        TAH->>RMQ: PUT /api/users/demo-consumer-{1,2,3} + permissions (management API)
        TAH-->>TAH: ready=true → /health returns 200
    end

    Note over RMQ,TAH: RabbitMQ HTTP authz backend now active —<br/>all authz decisions delegated to topic-auth-http

    RF->>Auth: POST /authentication/identity/login {robot-fleet}
    Auth-->>RF: {token: "…"}
    RF->>SR: POST /serviceregistry/register {telemetry, rabbitmq:5672, arrowhead exchange}
    SR-->>RF: 201 Created
    RF->>RMQ: AMQP connect (robot-fleet : fleet-secret)
    RMQ->>TAH: POST /auth/vhost  {username: robot-fleet, vhost: /}
    TAH-->>RMQ: allow  (publisher always allowed)
    RF->>RMQ: Exchange.Declare / queue setup
    RMQ->>TAH: POST /auth/resource  {username: robot-fleet, ...}
    TAH-->>RMQ: allow

    C->>Auth: POST /authentication/identity/login {demo-consumer-N}
    Auth-->>C: {token: "…"}
    C->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>Auth: GET /authentication/identity/verify [token] → verified demo-consumer-N
    DO->>SR: POST /serviceregistry/query {telemetry}
    SR-->>DO: [{provider: rabbitmq:5672, serviceUri: arrowhead, …}]
    DO->>CA: POST /authorization/verify {consumer-N, robot-fleet, telemetry}
    CA-->>DO: {authorized: true}
    DO-->>C: {provider: {address: rabbitmq, port: 5672}, metadata: {routingKeyPattern: telemetry.*}}
    C->>CA: POST /authorization/token/generate
    CA-->>C: {token: "…"}
    C->>RMQ: AMQP connect (demo-consumer-N : consumer-secret)
    RMQ->>TAH: POST /auth/vhost  {username: demo-consumer-N, vhost: /}
    TAH->>CA: GET /authorization/lookup  (live check)
    CA-->>TAH: [{consumer-N, telemetry}]
    TAH-->>RMQ: allow
    C->>RMQ: QueueBind(telemetry.#)
    RMQ->>TAH: POST /auth/topic  {username: demo-consumer-N, permission: read, routing_key: telemetry.#}
    TAH->>CA: GET /authorization/lookup  (live check)
    CA-->>TAH: [{consumer-N, telemetry}]
    TAH-->>RMQ: allow  (telemetry.# matches "telemetry" grant)
```

---

## Sequence Diagram 2 — Normal message flow

Once connected, robot-fleet publishes telemetry. RabbitMQ calls topic-auth-http for
every publish to verify the routing key. Messages are then delivered to subscribed
consumers without further authorization checks per delivery.

```mermaid
sequenceDiagram
    participant RF as robot-fleet
    participant RMQ as RabbitMQ<br/>(exchange: arrowhead)
    participant TAH as topic-auth-http
    participant C1 as consumer-1
    participant C2 as consumer-2
    participant C3 as consumer-3

    loop every ~100 ms per robot
        RF->>RMQ: Publish(key=telemetry.robot-1, payload=…)
        RMQ->>TAH: POST /auth/topic  {username: robot-fleet, permission: write, routing_key: telemetry.robot-1}
        TAH-->>RMQ: allow  (publisher write always allowed)
        RMQ->>C1: Deliver → demo-consumer-1-queue
        RMQ->>C2: Deliver → demo-consumer-2-queue
        RMQ->>C3: Deliver → demo-consumer-3-queue
    end
```

---

## Sequence Diagram 3 — Revoke and re-grant: dual-layer enforcement

Revoking a grant is effective immediately at the orchestration layer (DO returns
empty) and immediately at the broker layer when the consumer reconnects
(vhost authz check via topic-auth-http denies). An active idle subscriber is
disconnected within SYNC_INTERVAL (60 s) when the reconciliation sync deletes the user.

```mermaid
sequenceDiagram
    actor Operator
    participant CA as ConsumerAuth
    participant DO as DynamicOrch
    participant TAH as topic-auth-http
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

    Note over C2,RMQ: consumer-2 attempts to reconnect to broker directly (e.g., after AMQP drop)
    C2->>RMQ: AMQP connect (demo-consumer-2 : consumer-secret)
    Note over RMQ: authn: internal backend — user still exists → PASS
    RMQ->>TAH: POST /auth/vhost  {username: demo-consumer-2, vhost: /}
    TAH->>CA: GET /authorization/lookup  (live check)
    CA-->>TAH: []  (no grant for consumer-2)
    TAH-->>RMQ: deny
    RMQ-->>C2: Connection refused (ACCESS_REFUSED)
    Note over C2: immediate denial — no sync delay

    Note over TAH,RMQ: background reconciliation sync (≤ 60 s)
    TAH->>CA: GET /authorization/lookup
    Note over TAH: demo-consumer-2 absent → stale user
    TAH->>RMQ: DELETE /api/users/demo-consumer-2 (management API)
    Note over RMQ: active AMQP connection terminated (if still open)

    Operator->>CA: POST /authorization/grant {consumer-2, robot-fleet, telemetry}
    CA-->>Operator: 201 Created

    Note over TAH,RMQ: next reconciliation sync (≤ 60 s) re-creates user
    TAH->>CA: GET /authorization/lookup
    TAH->>RMQ: PUT /api/users/demo-consumer-2 + permissions (management API)

    C2->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>CA: POST /authorization/verify {consumer-2, robot-fleet, telemetry}
    CA-->>DO: {authorized: true}
    DO-->>C2: {provider: rabbitmq:5672, …}
    C2->>CA: POST /authorization/token/generate
    C2->>RMQ: AMQP connect → authn: internal PASS
    RMQ->>TAH: POST /auth/vhost  {username: demo-consumer-2}
    TAH->>CA: GET /authorization/lookup  (live check)
    CA-->>TAH: [{consumer-2, telemetry}]
    TAH-->>RMQ: allow
    C2->>RMQ: QueueBind(telemetry.#)
    RMQ->>TAH: POST /auth/topic  {username: demo-consumer-2, permission: read, routing_key: telemetry.#}
    TAH-->>RMQ: allow
    Note over C2,RMQ: consumer-2 resumes receiving messages
```

# Experiment 5 — Diagrams

## Component Diagram

Shows all services, their roles, and how they connect.

```mermaid
graph TD
    subgraph core["Arrowhead Core"]
        SR["ServiceRegistry :8080\n─────────────\nstores service\nregistrations"]
        AUTH["Authentication :8081\n─────────────\nissues and verifies\nBearer tokens"]
        CA["ConsumerAuth :8082\n─────────────\nstores authorization\ngrants in memory"]
        DO["DynamicOrch :8083\n─────────────\nverify identity\nquery SR + CA\nreturn endpoint"]
        CA_cert["CertAuth :8086\n─────────────\nissues X.509 certs\n(placeholder — not wired)"]
    end

    subgraph policy["Policy Engine"]
        PS["policy-sync :9095\n─────────────\npolls CA grants\ncompiles XACML PolicySet\nuploads to AuthzForce"]
        AZ["AuthzForce :8180\n─────────────\nXACML PDP/PAP\nsingle source of truth\nfor both transports"]
    end

    subgraph enforcement["Enforcement (PEPs)"]
        TAX["topic-auth-xacml :9090\n─────────────\nRabbitMQ HTTP authz\nbackend (AMQP PEP)\n→ AuthzForce"]
        KA["kafka-authz :9091\n─────────────\nKafka SSE proxy (PEP)\n→ AuthzForce\nSSE revocation events"]
    end

    subgraph transport["Transport Brokers"]
        RMQ["RabbitMQ :5672 / :15675\n─────────────\ntopic exchange: arrowhead\nauthz=HTTP backend"]
        KFK["Kafka :9092\n─────────────\ntopic: arrowhead.telemetry\n(internal only)"]
    end

    subgraph experiment["Experiment Services"]
        RF["robot-fleet :9105→9003\n─────────────\ndual-publish AMQP + Kafka\nregisters in SR"]
        C1["consumer-1\n(AMQP)"]
        C2["consumer-2\n(AMQP)"]
        C3["consumer-3\n(AMQP)"]
        AC["analytics-consumer\n─────────────\nKafka SSE subscriber"]
    end

    DB["Dashboard :3005\n─────────────\nhealth · grants · live data\npolicy projection tab"]

    PS  -->|"GET /authorization/lookup\n[every SYNC_INTERVAL]"| CA
    PS  -->|"PUT PolicySet\n(XACML 3.0, incremented version)"| AZ

    TAX -->|"POST /pdp/request\n(XACML decide)"| AZ
    KA  -->|"POST /pdp/request\n(XACML decide)"| AZ

    RMQ -->|"POST /auth/user\nPOST /auth/vhost\nPOST /auth/resource\nPOST /auth/topic"| TAX

    KA  -->|"subscribe\narrowhead.telemetry"| KFK
    KA  -.->|"SSE stream\n(authorized)"| AC

    RF  -->|"POST /serviceregistry/register\nat startup"| SR
    RF  -->|"POST /authentication/identity/login\nat startup"| AUTH
    RF  -->|"AMQP publish\ntelemetry.{robotId}"| RMQ
    RF  -->|"Kafka produce\nkey: telemetry.{robotId}"| KFK

    C1  -->|"POST /authentication/identity/login"| AUTH
    C1  -->|"POST /orchestration/dynamic\n[Bearer token]"| DO
    C1  -->|"POST /authorization/token/generate"| CA
    C1  -->|"AMQP subscribe"| RMQ

    C2  -->|"same AHC flow"| DO
    C3  -->|"same AHC flow"| DO

    DO  -->|"GET /authentication/identity/verify"| AUTH
    DO  -->|"POST /serviceregistry/query"| SR
    DO  -->|"POST /authorization/verify"| CA

    DB -.->|health + grants| CA
    DB -.->|health + status| PS
    DB -.->|management API| RMQ
    DB -.->|health + status| TAX
    DB -.->|health + status| KA
    DB -.->|health| SR
    DB -.->|health| AUTH
    DB -.->|health| DO
    DB -.->|health| CA_cert
    DB -.->|health + stats| RF
    DB -.->|health + stats| C1
    DB -.->|health + stats| C2
    DB -.->|health + stats| C3
    DB -.->|health + stats| AC

    style CA_cert fill:#f9fafb,stroke:#e5e7eb,color:#9ca3af
```

---

## Sequence Diagram 1 — Startup

`setup` seeds grants, `policy-sync` authenticates (optionally), creates the AuthzForce
domain, compiles the first PolicySet, and marks itself healthy, gating both PEPs.
`robot-fleet` and consumers start once the enforcement layer is ready.

```mermaid
sequenceDiagram
    actor Operator
    participant Setup
    participant SR as ServiceRegistry
    participant Auth as Authentication
    participant CA as ConsumerAuth
    participant DO as DynamicOrch
    participant PS as policy-sync
    participant AZ as AuthzForce
    participant TAX as topic-auth-xacml
    participant KA as kafka-authz
    participant RMQ as RabbitMQ
    participant KFK as Kafka
    participant RF as robot-fleet
    participant C as consumer-1/2/3
    participant AC as analytics-consumer

    Operator->>Setup: docker compose up

    Note over SR,DO: Core systems start in parallel

    Setup->>CA: POST /authorization/grant ×5
    Note over Setup: demo-consumer-{1,2,3}, analytics-consumer, test-probe
    CA-->>Setup: 201 Created ×5

    PS->>AZ: POST /domains  {description: arrowhead-exp5}
    AZ-->>PS: 200/201 {domainId}

    PS->>CA: GET /authorization/lookup
    CA-->>PS: [{demo-consumer-1,telemetry}, {demo-consumer-2,telemetry}, ...]

    PS->>AZ: PUT /domains/{id}/pap/pdp.xml  (XACML PolicySet v1)
    AZ-->>PS: 200 OK
    PS-->>PS: synced=true → /health returns 200

    Note over TAX,KA: Both PEPs wait for policy-sync to be healthy

    TAX->>AZ: (ready — will call /pdp/request on first broker event)
    KA->>KFK: (subscribes to arrowhead.telemetry — ready for SSE connections)

    RF->>Auth: POST /authentication/identity/login {robot-fleet}
    Auth-->>RF: {token: "…"}
    RF->>SR: POST /serviceregistry/register {telemetry, rabbitmq:5672, arrowhead exchange}
    SR-->>RF: 201 Created
    RF->>RMQ: AMQP connect (robot-fleet : fleet-secret)
    RMQ->>TAX: POST /auth/user  {username: robot-fleet}
    TAX-->>RMQ: allow  (publisher — hardcoded credentials)
    RMQ->>TAX: POST /auth/vhost  {username: robot-fleet, vhost: /}
    TAX-->>RMQ: allow  (publisher always allowed)

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
    RMQ->>TAX: POST /auth/user  {username: demo-consumer-N, password: consumer-secret}
    TAX->>AZ: POST /pdp/request  {subject: demo-consumer-N, resource: telemetry, action: subscribe}
    AZ-->>TAX: Permit
    TAX-->>RMQ: allow
    RMQ->>TAX: POST /auth/vhost  {username: demo-consumer-N, vhost: /}
    TAX->>AZ: POST /pdp/request  {subject: demo-consumer-N, resource: telemetry, action: subscribe}
    AZ-->>TAX: Permit
    TAX-->>RMQ: allow
    C->>RMQ: QueueBind(telemetry.#)
    RMQ->>TAX: POST /auth/topic  {username: demo-consumer-N, permission: read, routing_key: telemetry.#}
    TAX->>AZ: POST /pdp/request  {subject: demo-consumer-N, resource: telemetry, action: subscribe}
    AZ-->>TAX: Permit
    TAX-->>RMQ: allow

    AC->>KA: GET /stream/analytics-consumer?service=telemetry  (SSE)
    KA->>AZ: POST /pdp/request  {subject: analytics-consumer, resource: telemetry, action: subscribe}
    AZ-->>KA: Permit
    KA-->>AC: 200 OK (SSE headers flushed)
    Note over KA,AC: SSE stream open — messages forwarded from Kafka
```

---

## Sequence Diagram 2 — Normal message flow (dual-publish)

Once connected, `robot-fleet` publishes telemetry to both transports simultaneously.
RabbitMQ calls `topic-auth-xacml` on every publish. Kafka messages flow directly
to `kafka-authz`, which forwards them as SSE events to `analytics-consumer`.

```mermaid
sequenceDiagram
    participant RF as robot-fleet
    participant RMQ as RabbitMQ<br/>(exchange: arrowhead)
    participant TAX as topic-auth-xacml
    participant KFK as Kafka<br/>(topic: arrowhead.telemetry)
    participant KA as kafka-authz
    participant C1 as consumer-1
    participant C2 as consumer-2
    participant C3 as consumer-3
    participant AC as analytics-consumer

    loop every ~100 ms per robot
        RF->>RMQ: Publish(key=telemetry.robot-1, payload=…)
        RMQ->>TAX: POST /auth/topic  {username: robot-fleet, permission: write, routing_key: telemetry.robot-1}
        TAX-->>RMQ: allow  (publisher write always allowed)
        RMQ->>C1: Deliver → demo-consumer-1-queue
        RMQ->>C2: Deliver → demo-consumer-2-queue
        RMQ->>C3: Deliver → demo-consumer-3-queue

        RF->>KFK: Produce(key=telemetry.robot-1, payload=…)
        KFK->>KA: (internal Kafka delivery to subscriber)
        KA->>AC: SSE event: data: {…}
    end

    Note over KA,AC: kafka-authz re-checks AuthzForce every 100 messages
```

---

## Sequence Diagram 3 — Policy sync cycle

`policy-sync` runs every `SYNC_INTERVAL` (default 10 s). If grants change between
cycles, it uploads a new versioned PolicySet to AuthzForce. Both PEPs immediately
evaluate against the new policy on the next incoming request.

```mermaid
sequenceDiagram
    participant PS as policy-sync
    participant CA as ConsumerAuth
    participant AZ as AuthzForce<br/>(PAP + PDP)
    participant TAX as topic-auth-xacml
    participant KA as kafka-authz

    loop every SYNC_INTERVAL (10 s)
        PS->>CA: GET /authorization/lookup
        CA-->>PS: [{consumer-1,telemetry}, {consumer-2,telemetry}, …]
        Note over PS: compile grants → XACML 3.0 PolicySet<br/>increment version number
        PS->>AZ: PUT /domains/{id}/pap/pdp.xml  (PolicySet vN)
        AZ-->>PS: 200 OK  (active policy atomically updated)
        PS-->>PS: log "sync OK — version=N"
    end

    Note over TAX: next broker operation triggers /pdp/request<br/>evaluated against current active policy
    Note over KA: next revocation re-check (every 100 messages)<br/>evaluated against current active policy
```

---

## Sequence Diagram 4 — Revoke and re-grant: unified policy enforcement

Revoking a grant causes `policy-sync` to upload a new PolicySet within one sync
interval. Both PEPs (AMQP and Kafka) begin denying access on their next check —
without any per-transport configuration change.

```mermaid
sequenceDiagram
    actor Operator
    participant CA as ConsumerAuth
    participant PS as policy-sync
    participant AZ as AuthzForce
    participant DO as DynamicOrch
    participant TAX as topic-auth-xacml
    participant RMQ as RabbitMQ
    participant KA as kafka-authz
    participant C2 as consumer-2<br/>(AMQP)
    participant AC as analytics-consumer<br/>(Kafka SSE)

    Note over C2,RMQ: consumer-2 connected and receiving messages
    Note over AC,KA: analytics-consumer SSE stream open

    Operator->>CA: DELETE /authorization/revoke/{id}  (revoke analytics-consumer)
    CA-->>Operator: 200 OK

    Note over PS: next sync cycle (≤ 10 s)
    PS->>CA: GET /authorization/lookup
    CA-->>PS: [{consumer-1,telemetry}, {consumer-2,telemetry}, {consumer-3,telemetry}]
    Note over PS: analytics-consumer absent → removed from PolicySet
    PS->>AZ: PUT /domains/{id}/pap/pdp.xml  (PolicySet vN+1)
    AZ-->>PS: 200 OK

    Note over KA,AC: kafka-authz re-check at message 100N
    KA->>AZ: POST /pdp/request  {subject: analytics-consumer, resource: telemetry, action: subscribe}
    AZ-->>KA: Deny
    KA->>AC: SSE event: revoked\ndata: {"reason":"grant revoked"}
    Note over AC: disconnects and retries with exponential back-off

    AC->>KA: GET /stream/analytics-consumer?service=telemetry  (retry)
    KA->>AZ: POST /pdp/request  {subject: analytics-consumer, resource: telemetry, action: subscribe}
    AZ-->>KA: Deny
    KA-->>AC: 403 Forbidden
    Note over AC: back-off and retry

    Note over C2,RMQ: consumer-2 unaffected — its grant was not revoked

    Operator->>CA: POST /authorization/grant {analytics-consumer, robot-fleet, telemetry}
    CA-->>Operator: 201 Created

    Note over PS: next sync cycle (≤ 10 s)
    PS->>CA: GET /authorization/lookup
    CA-->>PS: [{consumer-1,…}, {consumer-2,…}, {consumer-3,…}, {analytics-consumer,telemetry}]
    PS->>AZ: PUT /domains/{id}/pap/pdp.xml  (PolicySet vN+2)
    AZ-->>PS: 200 OK

    AC->>KA: GET /stream/analytics-consumer?service=telemetry  (retry)
    KA->>AZ: POST /pdp/request  {subject: analytics-consumer, resource: telemetry, action: subscribe}
    AZ-->>KA: Permit
    KA-->>AC: 200 OK (SSE stream re-opened)
    Note over AC,KA: analytics-consumer resumes receiving messages

---

    Note over Operator: Separate scenario — revoke an AMQP consumer

    Operator->>CA: DELETE /authorization/revoke/{id}  (revoke demo-consumer-2)
    CA-->>Operator: 200 OK

    Note over PS: next sync cycle (≤ 10 s)
    PS->>AZ: PUT /domains/{id}/pap/pdp.xml  (PolicySet without demo-consumer-2)
    AZ-->>PS: 200 OK

    Note over C2: connection drops or next retry loop (≤ 5 s)
    C2->>DO: POST /orchestration/dynamic  [Bearer token]
    DO->>CA: POST /authorization/verify {consumer-2, robot-fleet, telemetry}
    CA-->>DO: {authorized: false}
    DO-->>C2: {response: []}  (empty — no authorized providers)
    Note over C2: "no authorized providers" → wait 5 s, retry

    Note over C2,RMQ: consumer-2 attempts to reconnect to broker
    C2->>RMQ: AMQP connect (demo-consumer-2 : consumer-secret)
    RMQ->>TAX: POST /auth/user  {username: demo-consumer-2, password: consumer-secret}
    TAX->>AZ: POST /pdp/request  {subject: demo-consumer-2, resource: telemetry, action: subscribe}
    AZ-->>TAX: Deny
    TAX-->>RMQ: deny
    RMQ-->>C2: Connection refused (ACCESS_REFUSED)
    Note over C2: denied immediately — policy already updated
```

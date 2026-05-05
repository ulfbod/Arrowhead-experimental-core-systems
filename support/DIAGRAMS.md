# Support — Diagrams

## Module Map

Shows all support modules, their kinds, and internal dependencies.

```mermaid
graph TD
    subgraph libs["Go Libraries (no server)"]
        MB["message-broker\n─────────────\nAMQP topic pub/sub\nabstraction"]
        AZL["authzforce (pkg)\n─────────────\nXACML REST client\nBuildPolicy · Decide"]
    end

    subgraph rmq_adapters["RabbitMQ Auth Adapters"]
        TAH["topic-auth-http :9090\n─────────────\nlive CA check\nper broker operation"]
        TAS["topic-auth-sync :9090\n─────────────\npolling sync\nRMQ user management"]
        TAX["topic-auth-xacml :9090\n─────────────\nXACML delegation\nper broker operation"]
    end

    subgraph xacml["XACML Policy Infrastructure"]
        AZS["authzforce-server :8080\n─────────────\nlightweight XACML PDP/PAP\n(Go — drop-in for AuthzForce CE)"]
        PS["policy-sync :9095\n─────────────\nCA grants → XACML PolicySet\npushed to AuthzForce"]
    end

    subgraph kafka["Kafka Enforcement"]
        KA["kafka-authz :9091\n─────────────\nKafka SSE proxy\nAuthorizes via AuthzForce"]
    end

    TAH -->|"GET /authorization/lookup\n[live per request]"| CA_ext["ConsumerAuth :8082\n(core)"]
    TAS -->|"GET /authorization/lookup\n[every SYNC_INTERVAL]"| CA_ext
    PS  -->|"GET /authorization/lookup\n[every SYNC_INTERVAL]"| CA_ext

    TAX -->|"POST /pdp"| AZS
    KA  -->|"POST /pdp"| AZS
    PS  -->|"PUT PolicySet"| AZS

    TAX -.->|imports| AZL
    KA  -.->|imports| AZL
    PS  -.->|imports| AZL

    style TAS fill:#f9fafb,stroke:#e5e7eb,color:#9ca3af
```

---

## Experiment-4 Context — Live HTTP Auth Backend

`topic-auth-http` is the sole auth backend for RabbitMQ. Every broker
operation triggers a live ConsumerAuth check. No polling delay — a revoked
grant is effective on the consumer's next operation.

```mermaid
graph LR
    CA["ConsumerAuth :8082"]
    TAH["topic-auth-http :9090\n─────────────\nHTTP auth backend"]
    RMQ["RabbitMQ :5672\nauth_backends.1=http"]

    CON["consumers\n(AMQP)"]
    PUB["robot-fleet\n(AMQP)"]

    RMQ -->|"POST /auth/user\nPOST /auth/vhost\nPOST /auth/resource\nPOST /auth/topic\n(every operation)"| TAH
    TAH -->|"GET /authorization/lookup\n[Bearer token]"| CA
    CON -->|"AMQP connect + subscribe"| RMQ
    PUB -->|"AMQP publish"| RMQ
```

---

## Experiment-5 Context — Unified XACML Policy Projection

`policy-sync` is the single writer: it compiles CA grants into a XACML
PolicySet and pushes it to AuthzForce. Both `topic-auth-xacml` (RabbitMQ) and
`kafka-authz` (Kafka SSE) query the same AuthzForce PDP — a single policy
governs both transports.

```mermaid
graph TD
    CA["ConsumerAuth :8082"]
    AZ["authzforce-server :8080\n─────────────\nXACML PDP/PAP"]

    PS["policy-sync :9095\n─────────────\nCA → XACML PolicySet\nSYNC_INTERVAL=30s"]

    TAX["topic-auth-xacml :9090\n─────────────\nRabbitMQ PEP"]
    KA["kafka-authz :9091\n─────────────\nKafka SSE PEP"]

    RMQ["RabbitMQ :5672"]
    KFK["Kafka :9092"]

    AMQP_CON["AMQP consumers"]
    KFK_CON["analytics-consumer\n(SSE)"]
    PUB["robot-fleet\n(dual publish)"]

    PS  -->|"GET /authorization/lookup\n[every 30 s]"| CA
    PS  -->|"PUT PolicySet\n(XACML 3.0)"| AZ

    RMQ -->|"POST /auth/*\n(every operation)"| TAX
    TAX -->|"POST /pdp\n(XACML decide)"| AZ

    KFK_CON -->|"GET /stream/{consumer}"| KA
    KA  -->|"POST /pdp\n(XACML decide)"| AZ
    KA  -->|"subscribe\narrowhead.telemetry"| KFK
    KA  -.->|"SSE messages\n(if Permit)"| KFK_CON

    AMQP_CON -->|"AMQP connect + subscribe"| RMQ
    PUB -->|"AMQP publish"| RMQ
    PUB -->|"Kafka produce"| KFK
```

---

## topic-auth-http — Decision Flow

One sequence per RabbitMQ operation type. The `handleResource` handler
always returns `allow`; fine-grained control is enforced at the topic level.

```mermaid
sequenceDiagram
    participant RMQ as RabbitMQ
    participant TAH as topic-auth-http
    participant Cache as Rules cache
    participant CA as ConsumerAuth

    Note over RMQ,CA: Consumer connects (AMQP)

    RMQ->>TAH: POST /auth/user  {username, password}
    TAH->>Cache: get rules
    alt cache miss
        Cache-->>TAH: miss
        TAH->>CA: GET /authorization/lookup [Bearer]
        CA-->>TAH: [{consumer, provider, service}, ...]
        TAH->>Cache: set rules (TTL)
    else cache hit
        Cache-->>TAH: rules
    end
    TAH-->>RMQ: allow  (password + any grant exists)

    RMQ->>TAH: POST /auth/vhost  {username}
    TAH->>Cache: get rules
    Cache-->>TAH: rules (likely still cached)
    TAH-->>RMQ: allow  (any grant exists)

    RMQ->>TAH: POST /auth/topic  {username, permission=read, routing_key}
    TAH->>Cache: get rules
    Cache-->>TAH: rules
    TAH-->>RMQ: allow / deny  (routing key matches service grant prefix)
```

---

## policy-sync — Sync Cycle

```mermaid
sequenceDiagram
    participant PS as policy-sync
    participant CA as ConsumerAuth
    participant AZ as authzforce-server

    loop every SYNC_INTERVAL
        PS->>CA: GET /authorization/lookup [Bearer]
        CA-->>PS: [{consumer, provider, service}, ...]
        PS->>PS: BuildPolicy(grants) → XACML PolicySet XML
        PS->>AZ: PUT /domains/{id}/pap/policies
        AZ-->>PS: 200 OK
        PS->>AZ: PUT /domains/{id}/pap/pdp.properties (root ref)
        AZ-->>PS: 200 OK
        PS->>PS: version++, lastSyncedAt = now
    end
```

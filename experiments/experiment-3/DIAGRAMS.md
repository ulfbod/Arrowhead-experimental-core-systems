# Experiment 3 — Diagrams

## Component Diagram

Shows the services, their roles, and how they connect.

```mermaid
graph TD
    subgraph core["Arrowhead Core"]
        CA["ConsumerAuth :8082\n─────────────\nstores authorization\ngrants in memory"]
    end

    subgraph support["Support Services"]
        TAS["topic-auth-sync :9090\n─────────────\npolls ConsumerAuth\nreconciles RabbitMQ\nusers & permissions"]
        RMQ["RabbitMQ :5672 / :15672\n─────────────\ntopic exchange: arrowhead\nauth_backend_topic plugin"]
    end

    subgraph experiment["Experiment Services"]
        RF["robot-fleet :9003\n─────────────\npublishes synthetic\ntelemetry to AMQP"]
        C1["consumer-1"]
        C2["consumer-2"]
        C3["consumer-3"]
    end

    DB["Dashboard :3003\n─────────────\nhealth · grants · live data"]

    TAS -->|"GET /authorization/lookup\nevery 10 s"| CA
    TAS -->|"Management API\nPUT users + permissions\nDELETE stale users"| RMQ

    RF -->|"AMQP publish\nrouting key: telemetry.{robotId}"| RMQ
    C1 -->|"AMQP subscribe\nbinding key: telemetry.#"| RMQ
    C2 -->|"AMQP subscribe\nbinding key: telemetry.#"| RMQ
    C3 -->|"AMQP subscribe\nbinding key: telemetry.#"| RMQ

    DB -.->|health + grants| CA
    DB -.->|management API| RMQ
    DB -.->|health| TAS
    DB -.->|health + stats| RF
    DB -.->|health + stats| C1
    DB -.->|health + stats| C2
    DB -.->|health + stats| C3
```

---

## Sequence Diagram 1 — Startup: provisioning users on first sync

`setup` seeds grants into ConsumerAuth. `topic-auth-sync` runs its first
reconciliation, creates all users and permissions in RabbitMQ, then marks
itself healthy. Only then do robot-fleet and consumers start.

```mermaid
sequenceDiagram
    actor Operator
    participant Setup
    participant CA as ConsumerAuth
    participant TAS as topic-auth-sync
    participant RMQ as RabbitMQ
    participant RF as robot-fleet
    participant C as consumer-1/2/3

    Operator->>Setup: docker compose up
    Setup->>CA: POST /authorization/grant ×3
    CA-->>Setup: 201 Created

    loop first sync cycle
        TAS->>CA: GET /authorization/lookup
        CA-->>TAS: [{consumer-1,telemetry}, {consumer-2,telemetry}, {consumer-3,telemetry}]
        TAS->>RMQ: PUT /api/users/robot-fleet  (tag: arrowhead-managed)
        TAS->>RMQ: PUT /api/permissions/%2F/robot-fleet  (configure/write/read: .*)
        TAS->>RMQ: PUT /api/topic-permissions/%2F/robot-fleet  (write: ^telemetry\.)
        TAS->>RMQ: PUT /api/users/demo-consumer-{1,2,3}
        TAS->>RMQ: PUT /api/permissions/%2F/demo-consumer-{1,2,3}
        TAS->>RMQ: PUT /api/topic-permissions/%2F/demo-consumer-{1,2,3}  (read: ^telemetry\.)
        TAS-->>TAS: ready=true → /health returns 200
    end

    RF->>RMQ: AMQP connect (robot-fleet : fleet-secret)
    RF->>RMQ: ExchangeDeclare(arrowhead, topic, durable)

    C->>RMQ: AMQP connect (demo-consumer-{n} : consumer-secret)
    C->>RMQ: QueueDeclare(demo-consumer-{n}-queue, durable)
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
        Note over RMQ: topic check write: ^telemetry\. matches telemetry.robot-1 ✔
        RMQ->>C1: Deliver → demo-consumer-1-queue
        RMQ->>C2: Deliver → demo-consumer-2-queue
        RMQ->>C3: Deliver → demo-consumer-3-queue
    end
```

---

## Sequence Diagram 3 — Revoke and re-grant: dynamic authorization

The core scenario this experiment demonstrates. Revoking a grant causes
`topic-auth-sync` to delete the RabbitMQ user, which forcibly closes the
consumer's AMQP connection. The consumer's retry loop resumes delivery as
soon as the grant is restored and the next sync cycle runs.

```mermaid
sequenceDiagram
    actor Operator
    participant CA as ConsumerAuth
    participant TAS as topic-auth-sync
    participant RMQ as RabbitMQ
    participant C2 as consumer-2

    Note over C2,RMQ: consumer-2 connected and receiving messages

    Operator->>CA: DELETE /authorization/revoke/{id}
    CA-->>Operator: 200 OK

    loop next sync cycle (≤ 10 s)
        TAS->>CA: GET /authorization/lookup
        CA-->>TAS: [{consumer-1, telemetry}, {consumer-3, telemetry}]
        Note over TAS: demo-consumer-2 absent → stale user
        TAS->>RMQ: DELETE /api/users/demo-consumer-2
    end

    RMQ->>C2: AMQP connection closed (403)
    Note over C2: retry loop: wait 3 s, attempt reconnect
    C2->>RMQ: AMQP connect → 403 username or password not allowed
    Note over C2: backs off, retries every 3 s — no messages received

    Operator->>CA: POST /authorization/grant  (consumer-2, robot-fleet, telemetry)
    CA-->>Operator: 201 Created

    loop next sync cycle (≤ 10 s)
        TAS->>CA: GET /authorization/lookup
        CA-->>TAS: [{consumer-1,telemetry}, {consumer-2,telemetry}, {consumer-3,telemetry}]
        TAS->>RMQ: PUT /api/users/demo-consumer-2
        TAS->>RMQ: PUT /api/permissions/%2F/demo-consumer-2
        TAS->>RMQ: PUT /api/topic-permissions/%2F/demo-consumer-2  (read: ^telemetry\.)
    end

    C2->>RMQ: AMQP connect → authenticated ✔
    C2->>RMQ: QueueBind(telemetry.#) — topic check: ^telemetry\. ✔
    Note over C2,RMQ: consumer-2 resumes receiving messages
```

# Experiment 3 — Direct AMQP Consumer Subscriptions with Topic-Based Authorization

This experiment replaces the edge-adapter HTTP bridge from experiment-2 with
direct AMQP subscriptions and enforces routing-key authorization at the broker
level using the `rabbitmq_auth_backend_topic` plugin. Authorization rules are
sourced from Arrowhead ConsumerAuthorization and continuously synced to
RabbitMQ by the `topic-auth-sync` service.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Arrowhead Core                                              │
│                                                              │
│  ┌────────────────┐        ┌──────────────────────────────┐ │
│  │ ConsumerAuth   │◄───────│  topic-auth-sync             │ │
│  │ :8082          │  poll  │  (syncs users + permissions) │ │
│  └────────────────┘        └──────────────┬───────────────┘ │
│                                            │ RabbitMQ        │
│                                            │ Management API  │
└────────────────────────────────────────────┼─────────────────┘
                                             │
                              ┌──────────────▼──────────────┐
                              │  RabbitMQ :5672             │
                              │  exchange: arrowhead        │
                              │  (topic + topic-auth plugin)│
                              └──────┬──────────────────────┘
                                     │
              ┌──────────────────────┼────────────────────────┐
              │                      │                        │
   ┌──────────▼───┐       ┌──────────▼───┐       ┌──────────▼───┐
   │ consumer-1   │       │ consumer-2   │       │ consumer-3   │
   │ (direct AMQP)│       │ (direct AMQP)│       │ (direct AMQP)│
   └──────────────┘       └──────────────┘       └──────────────┘
              ▲
   ┌──────────┴────────┐
   │  robot-fleet      │
   │  (AMQP publisher) │
   └───────────────────┘
```

See [DIAGRAMS.md](DIAGRAMS.md) for Mermaid component and sequence diagrams.

### Services

| Service | Port | Role |
|---|---|---|
| **consumerauth** | 8082 | Stores authorization grants; `topic-auth-sync` polls `/authorization/lookup` |
| **rabbitmq** | 5672 / 15673 | AMQP broker with `rabbitmq_auth_backend_topic` plugin; management UI on 15673 |
| **topic-auth-sync** | 9090 | Reconciles RabbitMQ users and topic permissions from ConsumerAuth every 10 s |
| **robot-fleet** | 9103 | Publishes synthetic telemetry to `telemetry.<robot-id>` routing keys |
| **consumer-1/2/3** | — | Subscribe directly to RabbitMQ using per-consumer credentials |

### Core systems used

| System | Used by |
|---|---|
| ConsumerAuthorization | `topic-auth-sync` reads grants to determine which consumers may bind which routing keys |

---

## Quick Start

```bash
cd experiments/experiment-3
docker compose up --build
```

Services start in dependency order:

```
rabbitmq → consumerauth → setup (seeds grants) → topic-auth-sync → robot-fleet + consumers
```

Open the dashboard at **http://localhost:3003**.

Watch consumer logs for live telemetry:

```bash
docker compose logs -f consumer-1
```

Stop everything:

```bash
docker compose down
```

---

## Key Concepts Demonstrated

### Broker-level routing-key authorization

Authorization is enforced at the AMQP protocol layer, not in application code.
When a consumer attempts to bind a queue to a routing key (e.g.
`telemetry.#`), RabbitMQ checks the topic permission configured for that user.
If the binding key does not match the allowed pattern, the broker rejects the
`Queue.Bind` operation — there is no application-layer bypass.

### Policy sync from Arrowhead ConsumerAuth

`topic-auth-sync` polls `GET /authorization/lookup` every 10 seconds and runs
a full reconciliation:

1. For each consumer found in the rules, create or update a RabbitMQ user
   tagged `arrowhead-managed` with a read-only topic permission pattern derived
   from the granted `serviceDefinition` values.
2. Delete any `arrowhead-managed` user that no longer appears in ConsumerAuth.

The mapping is: `serviceDefinition: "telemetry"` → topic read pattern
`^telemetry\.` (matches `telemetry.robot-1`, etc.).

### Permission pattern derivation

| serviceDefinition | Topic read pattern |
|---|---|
| `telemetry` | `^telemetry\.` |
| `sensors` | `^sensors\.` |
| `telemetry` + `sensors` (same consumer) | `^(sensors\|telemetry)\.` |

### Publisher permissions

`robot-fleet` is provisioned with write permission `^telemetry\.` — it can
only publish to routing keys starting with `telemetry.`, not to other prefixes.

---

## Verifying Authorization

**Authorized consumer (grant exists, bind succeeds):**
```bash
docker compose logs consumer-1
# Expect: "subscribed with binding key telemetry.#"
```

**Observe sync picking up a new grant:**
```bash
# Add a new grant via ConsumerAuth.
curl -X POST http://localhost:8082/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"consumerSystemName":"new-consumer","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}'

# Within ~10 s, topic-auth-sync creates user "new-consumer" in RabbitMQ.
# Check: RabbitMQ management UI → Admin → Users  (http://localhost:15673)
```

---

## Directory Structure

```
experiment-3/
├── docker-compose.yml
├── dockerfiles/
│   ├── core.Dockerfile              # shared Dockerfile for core binaries
│   ├── consumer-direct.Dockerfile
│   ├── robot-fleet.Dockerfile
│   └── topic-auth-sync.Dockerfile
├── rabbitmq/
│   ├── rabbitmq.conf                # enables rabbitmq_auth_backend_topic
│   └── enabled_plugins              # [rabbitmq_management, rabbitmq_auth_backend_topic]
└── services/
    └── consumer-direct/             # Go module: direct AMQP subscriber
```

The `topic-auth-sync` service lives at `support/topic-auth-sync/` (repo root)
and is referenced from `dockerfiles/topic-auth-sync.Dockerfile`.
The shared message broker library lives at `support/message-broker/` and is
referenced via `replace` directives in the service `go.mod` files.

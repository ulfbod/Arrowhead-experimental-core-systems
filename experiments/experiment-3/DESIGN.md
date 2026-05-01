# Experiment 3 — Direct AMQP Consumer Subscriptions with Topic-Based Authorization

## Problem

In experiment-2, consumers depend on the edge-adapter HTTP endpoint to receive
telemetry data. This has two limitations:

1. **Indirect delivery**: consumers poll or subscribe via HTTP; the edge-adapter
   acts as a bottleneck and adds latency.
2. **No broker-level authorization**: any AMQP client that can reach the broker
   can bind to any routing key; the Arrowhead ConsumerAuth policy is enforced
   only at the application layer.

## Solution

This experiment replaces the edge-adapter with direct AMQP subscriptions and
enforces authorization at the broker using the `rabbitmq_auth_backend_topic`
plugin. Policy rules are sourced from Arrowhead ConsumerAuth and continuously
synced to RabbitMQ topic permissions by the `topic-auth-sync` service.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Arrowhead Core                                              │
│                                                              │
│  ┌────────────────┐        ┌──────────────────────────────┐ │
│  │ ConsumerAuth   │◄───────│  topic-auth-sync             │ │
│  │ (policy store) │  poll  │  (syncs users+permissions)   │ │
│  └────────────────┘        └──────────────┬───────────────┘ │
│                                            │ RabbitMQ        │
│                                            │ Management API  │
└────────────────────────────────────────────┼─────────────────┘
                                             │
                              ┌──────────────▼──────────────┐
                              │  RabbitMQ                   │
                              │  exchange: arrowhead        │
                              │  (topic, topic-auth plugin) │
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

## Policy Mapping

| serviceDefinition | Routing key read pattern | Notes                          |
|-------------------|--------------------------|--------------------------------|
| `telemetry`       | `^telemetry\.`           | Matches `telemetry.robot-1` etc |
| `sensors`         | `^sensors\.`             | Matches `sensors.robot-1` etc   |
| `telemetry` + `sensors` (same consumer) | `^(sensors\|telemetry)\.` | Merged, sorted |

Publisher write pattern for `telemetry`: `^telemetry\.`

Consumer regular permissions (vhost): configure="", write="", read=".*"

Publisher regular permissions (vhost): configure=".*", write=".*", read=".*"

## Auth Flow

1. **ConsumerAuth** stores grant records: `{consumer, provider, serviceDefinition}`.
2. **topic-auth-sync** polls `GET /authorization/lookup` every 10 seconds.
3. For each consumer system found in the rules, topic-auth-sync:
   - Creates/updates a RabbitMQ user tagged `arrowhead-managed`.
   - Sets regular vhost permissions: read-only on queues.
   - Sets topic permission: `read=^{service}\.` on the `arrowhead` exchange.
4. Stale managed users (removed from ConsumerAuth) are deleted automatically.
5. **RabbitMQ** evaluates topic permissions at queue bind time using
   `rabbitmq_auth_backend_topic`. Unauthorized bind attempts are rejected.

## Security Properties

- An unauthorized consumer cannot bind a queue to `telemetry.#` — the broker
  rejects the bind at the AMQP protocol level.
- There is no application-layer bypass: the restriction lives in the broker's
  auth backend, not in service code.
- Consumers are provisioned with a shared password (`CONSUMER_PASSWORD`). In
  production this should be replaced with per-consumer credentials.
- The publisher (`robot-fleet`) is granted write permission only for routing
  keys matching `^telemetry\.`; it cannot publish to other prefixes.

## How to Run

```bash
cd experiments/experiment-3
docker compose up --build
```

Services start in order: rabbitmq → consumerauth → setup (seeds grants) →
topic-auth-sync (provisions users) → robot-fleet + consumers.

The robot-fleet control API is available at http://localhost:9103/config.
RabbitMQ management UI is at http://localhost:15673 (guest/guest).

## How to Verify Authorization

**Authorized consumer (succeeds):**
```bash
# demo-consumer-1 has a telemetry grant — binding works.
docker compose exec consumer-1 sh -c "echo connected"
# Check logs for "subscribed with binding key"
docker compose logs consumer-1
```

**Unauthorized consumer (rejected):**
```bash
# Attempt to subscribe without a valid grant using a different username/key.
# The broker will refuse the bind because no topic permission matches.
docker run --rm --network experiment-3_default \
  pivotalrabbitmq/perf-test:latest \
  --uri "amqp://unknown-user:wrong-pass@rabbitmq:5672/" \
  --queue test-unauthorized \
  --routing-key "telemetry.#" \
  --consumers 1 --producers 0
# Expected: connection or permission error
```

**Observe topic-auth-sync syncing a new grant:**
```bash
# Add a new grant via ConsumerAuth.
curl -X POST http://localhost:8082/authorization/grant \
  -H 'Content-Type: application/json' \
  -d '{"consumerSystemName":"new-consumer","providerSystemName":"robot-fleet","serviceDefinition":"telemetry"}'

# Within ~10s, topic-auth-sync creates user "new-consumer" in RabbitMQ.
# Check RabbitMQ management UI → Admin → Users.
```

---
name: QoS failover evaluation implementation
description: What was added for QoS-aware failover delay experiments (network delay, orchestration modes, CSV output)
type: project
---

QoS-aware failover evaluation system added (2026-03-29). New capabilities:

**Why:** Academic paper needs experimental evaluation of failover delay vs network latency for local vs centralized orchestration modes.

**How to apply:** When working on failover/QoS experiments, these are the key components:

- `common/client.go`: Global `SetNetworkDelayMs(ms)` / `GetNetworkDelayMs()` and `SetOrchestrationMode("local"|"central")` / `GetOrchestrationMode()`. Network delay is injected into every `DoRequest` call.
- `common/qos.go`: `FailoverEvent` now has `DecisionDelayMs` (detection→switch, key metric), `OrchestrationMode`, `NetworkDelayMs`.
- `common/provider.go`: `ProviderSelector` now takes `*ArrowheadClient` as last param. In "central" mode calls Arrowhead to discover fallback (adds 2×networkDelay overhead). `discoverViaCentral()` handles fallback. `LatestFailoverEvent()` added.
- `common/qoslog.go`: `FailoverLogger` CSV now includes `decision_delay_ms`, `orchestration_mode`, `network_delay_ms`.
- `cdt1/main.go` and `cdt2/main.go`: Pass `ah` to `NewProviderSelector`. Added `POST /trigger-poll` endpoint (for experiment runner to force immediate poll cycle).
- `scenario/main.go`: New endpoints:
  - `POST /scenario/config?mode=local|central`
  - `POST /scenario/network-delay?ms=X`
  - `POST /scenario/experiment/run` (async, body: `{"runsPerPoint":5}`)
  - `GET /scenario/experiment/results`
  - Writes `failover_delay_vs_network_delay.csv` (gnuplot-compatible)
- Frontend: New "QoS & Failover" tab (`src/components/QoSView/index.tsx`) with provider health, failover history, experiment controls, results table.

**Experiment design:** For each (networkDelay 0-50ms step 5, mode local/central):
1. Recover idt2a + cDT2 provider (at 0ms delay for clean reset)
2. Set networkDelay + mode
3. `POST idt2a/simulate/fail` (503 responses)
4. `POST cdt2/provider/fail {"providerId":"idt2a"}` (pre-sets consecFails >= threshold)
5. `POST cdt2/trigger-poll` → fires failover immediately, returns FailoverEvent
6. Record `DecisionDelayMs` (SwitchTime - DetectionTime) = key metric

**CSV output:** `logs/failover_delay_vs_network_delay.csv` — headers: `network_delay_ms,failover_delay_local_ms,failover_delay_central_ms`. Directly usable with gnuplot.

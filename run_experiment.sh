#!/usr/bin/env bash
# run_experiment.sh  –  Start all services, run the failover experiment, collect results.
#
# Runs everything locally with "go run" (no Docker required).
#
# Usage:
#   ./run_experiment.sh                   # default: 5 runs per data point
#   ./run_experiment.sh --runs 10         # more runs for better averaging
#   ./run_experiment.sh --no-start        # skip starting services (already running)
#   ./run_experiment.sh --help
#
# Output:
#   logs/failover_delay_vs_network_delay.csv  ← gnuplot-ready aggregated CSV
#   logs/failover_events.csv                  ← every individual failover event
#   logs/cdt2_gas_stream.csv                  ← per-poll sensor QoS stream

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
RUNS_PER_POINT=5
DO_START=true
SCENARIO_URL="http://localhost:8700"
TIMEOUT_STARTUP=60    # seconds to wait for each service
TIMEOUT_EXPERIMENT=600 # seconds to wait for experiment completion

# ── Argument parsing ────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --runs)     RUNS_PER_POINT="$2"; shift 2 ;;
    --no-start) DO_START=false; shift ;;
    --help|-h)
      grep '^#' "$0" | sed 's/^# \{0,2\}//'
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Colours ─────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info() { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()  { echo -e "${RED}[ERR ]${NC}  $*" >&2; exit 1; }

# ── Prerequisites ────────────────────────────────────────────────────────────
command -v go   >/dev/null 2>&1 || die "go not found – install Go 1.21+"
command -v curl >/dev/null 2>&1 || die "curl not found"
command -v jq   >/dev/null 2>&1 && HAS_JQ=true || HAS_JQ=false

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND="$SCRIPT_DIR/backend"
LOG_DIR="$SCRIPT_DIR/logs"
mkdir -p "$LOG_DIR"

PIDS=()

cleanup() {
  if [[ ${#PIDS[@]} -gt 0 ]]; then
    info "Stopping background services…"
    for pid in "${PIDS[@]}"; do
      kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    ok "Services stopped."
  fi
}
# Only register cleanup if we started services ourselves
[[ "$DO_START" == true ]] && trap cleanup EXIT INT TERM

# ── Helper: wait for an HTTP endpoint ───────────────────────────────────────
wait_for() {
  local label="$1" url="$2"
  local elapsed=0 interval=2
  printf "  Waiting for %-22s " "$label…"
  while ! curl -sf --max-time 2 "$url" >/dev/null 2>&1; do
    sleep $interval
    elapsed=$((elapsed + interval))
    printf "."
    if [[ $elapsed -ge $TIMEOUT_STARTUP ]]; then
      echo ""
      die "$label did not respond within ${TIMEOUT_STARTUP}s (tried $url)"
    fi
  done
  echo -e "  ${GREEN}up${NC}"
}

# ── Helper: start one Go service in the background ──────────────────────────
start_svc() {
  local label="$1"; shift
  # remaining args are env=val pairs followed by the go package path
  env "$@" go run "$BACKEND/$(grep -oP '(?<=./cmd/)\S+' <<< "$*" || true)"
  # Actually just run it directly:
  env "$@" &
  PIDS+=($!)
  info "Started $label (pid ${PIDS[-1]})"
}

# ── 1. Start all services ────────────────────────────────────────────────────
if [[ "$DO_START" == true ]]; then
  echo ""
  info "Building and starting all services with 'go run'…"
  info "(First run compiles everything – may take ~30s)"
  echo ""

  cd "$BACKEND"

  PORT=8000 go run ./cmd/arrowhead &
  PIDS+=($!); info "arrowhead       :8000"
  sleep 3  # arrowhead must be up before others register

  PORT=8101 IDT_ID=idt1a IDT_NAME="Inspection Robot A" ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-robot &
  PIDS+=($!); info "idt1a           :8101"

  PORT=8102 IDT_ID=idt1b IDT_NAME="Inspection Robot B" ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-robot &
  PIDS+=($!); info "idt1b           :8102"

  PORT=8201 IDT_ID=idt2a IDT_NAME="Gas Sensing Unit A" ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-gas &
  PIDS+=($!); info "idt2a           :8201"

  PORT=8202 IDT_ID=idt2b IDT_NAME="Gas Sensing Unit B" ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-gas &
  PIDS+=($!); info "idt2b           :8202"

  PORT=8301 IDT_ID=idt3a IDT_NAME="LHD Vehicle A"      ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-lhd &
  PIDS+=($!); info "idt3a           :8301"

  PORT=8302 IDT_ID=idt3b IDT_NAME="LHD Vehicle B"      ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-lhd &
  PIDS+=($!); info "idt3b           :8302"

  PORT=8401 IDT_ID=idt4  IDT_NAME="Tele-Remote"        ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-teleremote &
  PIDS+=($!); info "idt4            :8401"

  sleep 3

  PORT=8501 ARROWHEAD_URL=http://localhost:8000 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    LOG_DIR="$LOG_DIR" go run ./cmd/cdt1 &
  PIDS+=($!); info "cdt1            :8501"

  PORT=8502 ARROWHEAD_URL=http://localhost:8000 \
    IDT2A_URL=http://localhost:8201 IDT2B_URL=http://localhost:8202 \
    LOG_DIR="$LOG_DIR" go run ./cmd/cdt2 &
  PIDS+=($!); info "cdt2            :8502"

  PORT=8503 ARROWHEAD_URL=http://localhost:8000 \
    CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    go run ./cmd/cdt3 &
  PIDS+=($!); info "cdt3            :8503"

  PORT=8504 ARROWHEAD_URL=http://localhost:8000 \
    IDT3A_URL=http://localhost:8301 IDT3B_URL=http://localhost:8302 \
    go run ./cmd/cdt4 &
  PIDS+=($!); info "cdt4            :8504"

  PORT=8505 ARROWHEAD_URL=http://localhost:8000 \
    IDT4_URL=http://localhost:8401 \
    go run ./cmd/cdt5 &
  PIDS+=($!); info "cdt5            :8505"

  sleep 3

  PORT=8601 ARROWHEAD_URL=http://localhost:8000 \
    CDT1_URL=http://localhost:8501 CDT3_URL=http://localhost:8503 \
    CDT4_URL=http://localhost:8504 CDT5_URL=http://localhost:8505 \
    LOG_DIR="$LOG_DIR" go run ./cmd/cdta &
  PIDS+=($!); info "cdta            :8601"

  PORT=8602 ARROWHEAD_URL=http://localhost:8000 \
    CDT2_URL=http://localhost:8502 CDT3_URL=http://localhost:8503 \
    LOG_DIR="$LOG_DIR" go run ./cmd/cdtb &
  PIDS+=($!); info "cdtb            :8602"

  sleep 2

  PORT=8700 ARROWHEAD_URL=http://localhost:8000 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    IDT2A_URL=http://localhost:8201 IDT2B_URL=http://localhost:8202 \
    IDT3A_URL=http://localhost:8301 IDT3B_URL=http://localhost:8302 \
    IDT4_URL=http://localhost:8401 \
    CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
    CDT3_URL=http://localhost:8503 CDT4_URL=http://localhost:8504 \
    CDT5_URL=http://localhost:8505 \
    CDTA_URL=http://localhost:8601 CDTB_URL=http://localhost:8602 \
    LOG_DIR="$LOG_DIR" go run ./cmd/scenario &
  PIDS+=($!); info "scenario        :8700"

  # ── Frontend (Vite dev server) ──────────────────────────────────────────────
  FRONTEND="$SCRIPT_DIR/frontend"
  if [[ -d "$FRONTEND/node_modules" ]]; then
    cd "$FRONTEND"
    npm run dev -- --port 3000 --host &
    PIDS+=($!); info "frontend        :3000  (http://localhost:3000)"
    cd "$BACKEND"
  else
    warn "frontend/node_modules not found – run 'npm install' in $FRONTEND first"
    warn "Skipping frontend startup; run it manually with: cd $FRONTEND && npm run dev"
  fi
fi

# ── 2. Wait for key services to be healthy ───────────────────────────────────
echo ""
info "Waiting for services to be healthy…"
wait_for "arrowhead"       "http://localhost:8000/registry"
wait_for "idt2a"           "http://localhost:8201/health"
wait_for "idt2b"           "http://localhost:8202/health"
wait_for "cdt1"            "http://localhost:8501/health"
wait_for "cdt2"            "http://localhost:8502/health"
wait_for "scenario runner" "http://localhost:8700/health"

info "Waiting 5s for Arrowhead registration to settle…"
sleep 5

# ── 3. Launch the experiment ─────────────────────────────────────────────────
TOTAL_RUNS=$(( 11 * 2 * RUNS_PER_POINT ))
echo ""
info "Launching failover experiment"
info "  runsPerPoint = $RUNS_PER_POINT"
info "  total runs   = $TOTAL_RUNS  (11 delays × 2 modes × $RUNS_PER_POINT)"
info "  est. time    = ~$((TOTAL_RUNS / 3))–$((TOTAL_RUNS))s"

RESP=$(curl -sf -X POST "$SCENARIO_URL/scenario/experiment/run" \
  -H "Content-Type: application/json" \
  -d "{\"runsPerPoint\": $RUNS_PER_POINT}")
echo "$RESP"

# ── 4. Poll until done ───────────────────────────────────────────────────────
echo ""
info "Progress (polling every 5s)…"
elapsed=0
while true; do
  STATUS=$(curl -sf "$SCENARIO_URL/scenario/experiment/results" 2>/dev/null || echo '{"status":"unreachable"}')
  STATE=$(echo "$STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','?'))" 2>/dev/null \
          || echo "$STATUS" | grep -oP '"status"\s*:\s*"\K[^"]+' || echo "?")

  case "$STATE" in
    completed)  echo ""; ok "Experiment complete!"; break ;;
    running)    printf "." ;;
    unreachable) printf "?" ;;
    *)          printf "[$STATE]" ;;
  esac

  sleep 5
  elapsed=$((elapsed + 5))
  [[ $elapsed -ge $TIMEOUT_EXPERIMENT ]] && die "Timed out after ${TIMEOUT_EXPERIMENT}s"
done

# ── 5. Print results ─────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  RESULTS  –  Decision delay (detection → switch)${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

RESULTS=$(curl -sf "$SCENARIO_URL/scenario/experiment/results")

if $HAS_JQ; then
  echo "$RESULTS" | jq -r '
    ["delay_ms","local_ms","central_ms"],
    (.results.summary[] | [.networkDelayMs, (.avgLocalDecisionMs|.*100|round/100), (.avgCentralDecisionMs|.*100|round/100)])
    | @tsv' | column -t
else
  echo "$RESULTS" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'{'delay_ms':>10}  {'local_ms':>10}  {'central_ms':>12}')
for s in d.get('results', {}).get('summary', []):
    print(f\"{s['networkDelayMs']:>10}  {s['avgLocalDecisionMs']:>10.1f}  {s['avgCentralDecisionMs']:>12.1f}\")
" 2>/dev/null || echo "$RESULTS"
fi

# ── 6. Show CSV ──────────────────────────────────────────────────────────────
CSV="$LOG_DIR/failover_delay_vs_network_delay.csv"
echo ""
if [[ -f "$CSV" ]]; then
  ok "Aggregated CSV written to: $CSV"
  echo ""
  cat "$CSV"
else
  warn "CSV not found at $CSV"
fi

# ── 7. gnuplot hint ───────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}To plot with gnuplot (errorbars = p10/p90):${NC}"
echo "  set xlabel 'Network Delay (ms)'"
echo "  set ylabel 'Failover Decision Delay (ms)'"
echo "  plot \"$CSV\" using 1:2:3:4 with yerrorbars title 'Local', \\"
echo "       \"\" using 1:2 with linespoints notitle lc 1, \\"
echo "       \"\" using 1:5:6:7 with yerrorbars title 'Centralized', \\"
echo "       \"\" using 1:5 with linespoints notitle lc 2"

echo ""
echo -e "${BLUE}Other log files in $LOG_DIR/:${NC}"
for f in failover_events.csv cdt2_gas_stream.csv cdt1_mapping_stream.csv; do
  [[ -f "$LOG_DIR/$f" ]] && ok "  $f" || warn "  $f (not yet written)"
done

if [[ "$DO_START" == true ]]; then
  echo ""
  info "Services still running. Press Ctrl+C to stop them."
  info "Frontend: http://localhost:3000"
  wait
fi

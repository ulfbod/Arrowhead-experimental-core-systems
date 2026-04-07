#!/usr/bin/env bash
# start.sh — Start the full MineIO Digital Twin system (backend + frontend)
#
# Starts all Go backend services and the Vite frontend dev server.
# Press Ctrl+C to stop everything cleanly.
#
# Usage:
#   ./start.sh              # normal start
#   ./start.sh --frontend-only   # skip Go services (uncertainty sim tab only)
#   ./start.sh --port 3001       # custom frontend port (default: 3000)
#   ./start.sh --no-open         # don't try to open the browser

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
FRONTEND_ONLY=false
FRONTEND_PORT=3000
OPEN_BROWSER=true
TIMEOUT_STARTUP=90   # seconds to wait for each service

# ── Args ──────────────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --frontend-only) FRONTEND_ONLY=true; shift ;;
    --port)          FRONTEND_PORT="$2"; shift 2 ;;
    --no-open)       OPEN_BROWSER=false; shift ;;
    --help|-h)
      grep '^#' "$0" | sed 's/^# \{0,2\}//'
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
info() { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()  { echo -e "${RED}[ERR ]${NC}  $*" >&2; exit 1; }

# ── Paths ─────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND="$SCRIPT_DIR/backend"
FRONTEND="$SCRIPT_DIR/frontend"
LOGS="$SCRIPT_DIR/logs"
mkdir -p "$LOGS"

# ── PID tracking and cleanup ──────────────────────────────────────────────────
PIDS=()

cleanup() {
  echo ""
  info "Shutting down…"
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  ok "All services stopped."
}
trap cleanup EXIT INT TERM

# ── Prerequisites ─────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  MineIO Digital Twin — System Start${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

command -v node >/dev/null 2>&1 || die "node not found — install Node.js 18+"
command -v npm  >/dev/null 2>&1 || die "npm not found"

if [[ "$FRONTEND_ONLY" == false ]]; then
  command -v go >/dev/null 2>&1 || die "go not found — install Go 1.21+"
fi

# ── Frontend dependencies ─────────────────────────────────────────────────────
if [[ ! -d "$FRONTEND/node_modules" ]]; then
  info "Installing frontend dependencies (first time only)…"
  cd "$FRONTEND" && npm install --silent && cd "$SCRIPT_DIR"
  ok "npm install done."
fi

# ── Health-check helper ───────────────────────────────────────────────────────
wait_for() {
  local label="$1" url="$2"
  local elapsed=0 interval=2
  printf "  %-30s" "Waiting for $label…"
  while ! curl -sf --max-time 2 "$url" >/dev/null 2>&1; do
    sleep $interval
    elapsed=$((elapsed + interval))
    printf "."
    if [[ $elapsed -ge $TIMEOUT_STARTUP ]]; then
      echo ""
      die "$label did not respond within ${TIMEOUT_STARTUP}s (url: $url)"
    fi
  done
  echo -e "  ${GREEN}up${NC}"
}

# ─────────────────────────────────────────────────────────────────────────────
# BACKEND SERVICES (skipped with --frontend-only)
# ─────────────────────────────────────────────────────────────────────────────
if [[ "$FRONTEND_ONLY" == false ]]; then
  info "Starting Go backend services…"
  info "(First run compiles all services — may take ~30 s)"
  echo ""

  cd "$BACKEND"

  # ── Layer 0: Arrowhead core ─────────────────────────────────────────────────
  PORT=8000 go run ./cmd/arrowhead \
    >> "$LOGS/arrowhead.log" 2>&1 &
  PIDS+=($!); info "arrowhead       :8000"
  sleep 3   # Arrowhead must be up before others register

  # ── Layer 1: Individual Digital Twins ───────────────────────────────────────
  PORT=8101 IDT_ID=idt1a IDT_NAME="Inspection Robot A" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-robot >> "$LOGS/idt1a.log" 2>&1 &
  PIDS+=($!); info "idt1a           :8101"

  PORT=8102 IDT_ID=idt1b IDT_NAME="Inspection Robot B" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-robot >> "$LOGS/idt1b.log" 2>&1 &
  PIDS+=($!); info "idt1b           :8102"

  PORT=8201 IDT_ID=idt2a IDT_NAME="Gas Sensing Unit A" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-gas >> "$LOGS/idt2a.log" 2>&1 &
  PIDS+=($!); info "idt2a           :8201"

  PORT=8202 IDT_ID=idt2b IDT_NAME="Gas Sensing Unit B" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-gas >> "$LOGS/idt2b.log" 2>&1 &
  PIDS+=($!); info "idt2b           :8202"

  PORT=8301 IDT_ID=idt3a IDT_NAME="LHD Vehicle A" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-lhd >> "$LOGS/idt3a.log" 2>&1 &
  PIDS+=($!); info "idt3a           :8301"

  PORT=8302 IDT_ID=idt3b IDT_NAME="LHD Vehicle B" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-lhd >> "$LOGS/idt3b.log" 2>&1 &
  PIDS+=($!); info "idt3b           :8302"

  PORT=8401 IDT_ID=idt4 IDT_NAME="Tele-Remote" ARROWHEAD_URL=http://localhost:8000 \
    go run ./cmd/idt-teleremote >> "$LOGS/idt4.log" 2>&1 &
  PIDS+=($!); info "idt4            :8401"

  sleep 3

  # ── Layer 2: Lower Composite Digital Twins ───────────────────────────────────
  PORT=8501 ARROWHEAD_URL=http://localhost:8000 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    LOG_DIR="$LOGS" go run ./cmd/cdt1 >> "$LOGS/cdt1.log" 2>&1 &
  PIDS+=($!); info "cdt1 (mapping)  :8501"

  PORT=8502 ARROWHEAD_URL=http://localhost:8000 \
    IDT2A_URL=http://localhost:8201 IDT2B_URL=http://localhost:8202 \
    LOG_DIR="$LOGS" go run ./cmd/cdt2 >> "$LOGS/cdt2.log" 2>&1 &
  PIDS+=($!); info "cdt2 (gas)      :8502"

  PORT=8503 ARROWHEAD_URL=http://localhost:8000 \
    CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    go run ./cmd/cdt3 >> "$LOGS/cdt3.log" 2>&1 &
  PIDS+=($!); info "cdt3 (hazard)   :8503"

  PORT=8504 ARROWHEAD_URL=http://localhost:8000 \
    IDT3A_URL=http://localhost:8301 IDT3B_URL=http://localhost:8302 \
    go run ./cmd/cdt4 >> "$LOGS/cdt4.log" 2>&1 &
  PIDS+=($!); info "cdt4 (clearance):8504"

  PORT=8505 ARROWHEAD_URL=http://localhost:8000 \
    IDT4_URL=http://localhost:8401 \
    go run ./cmd/cdt5 >> "$LOGS/cdt5.log" 2>&1 &
  PIDS+=($!); info "cdt5 (tele-rem) :8505"

  sleep 3

  # ── Layer 3: Upper Composite Digital Twins ───────────────────────────────────
  PORT=8601 ARROWHEAD_URL=http://localhost:8000 \
    CDT1_URL=http://localhost:8501 CDT3_URL=http://localhost:8503 \
    CDT4_URL=http://localhost:8504 CDT5_URL=http://localhost:8505 \
    LOG_DIR="$LOGS" go run ./cmd/cdta >> "$LOGS/cdta.log" 2>&1 &
  PIDS+=($!); info "cdta (mission)  :8601"

  PORT=8602 ARROWHEAD_URL=http://localhost:8000 \
    CDT2_URL=http://localhost:8502 CDT3_URL=http://localhost:8503 \
    LOG_DIR="$LOGS" go run ./cmd/cdtb >> "$LOGS/cdtb.log" 2>&1 &
  PIDS+=($!); info "cdtb (hazard mon):8602"

  sleep 2

  # ── Scenario runner ──────────────────────────────────────────────────────────
  PORT=8700 ARROWHEAD_URL=http://localhost:8000 \
    IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
    IDT2A_URL=http://localhost:8201 IDT2B_URL=http://localhost:8202 \
    IDT3A_URL=http://localhost:8301 IDT3B_URL=http://localhost:8302 \
    IDT4_URL=http://localhost:8401 \
    CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
    CDT3_URL=http://localhost:8503 CDT4_URL=http://localhost:8504 \
    CDT5_URL=http://localhost:8505 \
    CDTA_URL=http://localhost:8601 CDTB_URL=http://localhost:8602 \
    LOG_DIR="$LOGS" go run ./cmd/scenario >> "$LOGS/scenario.log" 2>&1 &
  PIDS+=($!); info "scenario runner :8700"

  cd "$SCRIPT_DIR"
fi

# ─────────────────────────────────────────────────────────────────────────────
# FRONTEND
# ─────────────────────────────────────────────────────────────────────────────
echo ""
info "Starting frontend (Vite dev server on :${FRONTEND_PORT})…"
cd "$FRONTEND"
npm run dev -- --port "$FRONTEND_PORT" --host \
  >> "$LOGS/frontend.log" 2>&1 &
PIDS+=($!); info "frontend        :${FRONTEND_PORT}"
cd "$SCRIPT_DIR"

# ─────────────────────────────────────────────────────────────────────────────
# HEALTH CHECKS
# ─────────────────────────────────────────────────────────────────────────────
echo ""
info "Waiting for services to come up…"

if [[ "$FRONTEND_ONLY" == false ]]; then
  wait_for "arrowhead"        "http://localhost:8000/registry"
  wait_for "idt2a"            "http://localhost:8201/health"
  wait_for "idt2b"            "http://localhost:8202/health"
  wait_for "cdt1"             "http://localhost:8501/health"
  wait_for "cdt2"             "http://localhost:8502/health"
  wait_for "scenario runner"  "http://localhost:8700/health"
fi

wait_for "frontend" "http://localhost:${FRONTEND_PORT}"

# ─────────────────────────────────────────────────────────────────────────────
# READY
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}${BOLD}  System is ready${NC}"
echo -e "${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  Frontend  →  ${BOLD}http://localhost:${FRONTEND_PORT}${NC}"
echo ""
echo -e "  Tabs available:"
echo -e "    • System View"
echo -e "    • cDTa: Inspection & Recovery"
echo -e "    • cDTb: Hazard Monitoring"
echo -e "    • QoS & Failover"
echo -e "    • Simulation"
echo -e "    • ${GREEN}Uncertainty-Aware Simulation${NC}  (runs in-browser, no backend needed)"
echo ""
echo -e "  Service logs: ${LOGS}/"
echo ""
echo -e "  Press ${BOLD}Ctrl+C${NC} to stop everything."
echo ""

# Try to open browser
if [[ "$OPEN_BROWSER" == true ]]; then
  URL="http://localhost:${FRONTEND_PORT}"
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$URL" 2>/dev/null || true
  elif command -v open >/dev/null 2>&1; then
    open "$URL" 2>/dev/null || true
  fi
fi

# Block until Ctrl+C
wait

#!/usr/bin/env bash
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND="$REPO_ROOT/backend"
FRONTEND="$REPO_ROOT/frontend"
PIDS=()

cleanup() {
  echo "Stopping all services..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait
  echo "All services stopped."
}
trap cleanup EXIT INT TERM

cd "$BACKEND"

# ── Arrowhead Core ─────────────────────────────────────────────────────────────
PORT=8000 go run ./cmd/arrowhead &
PIDS+=($!)
echo "Started Arrowhead on :8000"
sleep 2

# ── iDT Layer ──────────────────────────────────────────────────────────────────
IDT_ID=idt1a IDT_NAME="Inspection Robot A" PORT=8101 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-robot &
PIDS+=($!)
IDT_ID=idt1b IDT_NAME="Inspection Robot B" PORT=8102 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-robot &
PIDS+=($!)
IDT_ID=idt2a IDT_NAME="Gas Sensing Unit A"  PORT=8201 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-gas &
PIDS+=($!)
IDT_ID=idt2b IDT_NAME="Gas Sensing Unit B"  PORT=8202 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-gas &
PIDS+=($!)
IDT_ID=idt3a IDT_NAME="LHD Vehicle A"       PORT=8301 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-lhd &
PIDS+=($!)
IDT_ID=idt3b IDT_NAME="LHD Vehicle B"       PORT=8302 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-lhd &
PIDS+=($!)
IDT_ID=idt4  IDT_NAME="Tele-Remote Station" PORT=8401 ARROWHEAD_URL=http://localhost:8000 go run ./cmd/idt-teleremote &
PIDS+=($!)
echo "Started 7 iDT services"
sleep 3

# ── Lower cDT Layer ────────────────────────────────────────────────────────────
PORT=8501 ARROWHEAD_URL=http://localhost:8000 \
  IDT1B_URL=http://localhost:8102 \
  go run ./cmd/cdt1 &
PIDS+=($!)

PORT=8502 ARROWHEAD_URL=http://localhost:8000 \
  IDT2B_URL=http://localhost:8202 \
  go run ./cmd/cdt2 &
PIDS+=($!)

PORT=8503 ARROWHEAD_URL=http://localhost:8000 \
  CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
  IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
  go run ./cmd/cdt3 &
PIDS+=($!)

PORT=8504 ARROWHEAD_URL=http://localhost:8000 \
  IDT3B_URL=http://localhost:8302 \
  go run ./cmd/cdt4 &
PIDS+=($!)

PORT=8505 ARROWHEAD_URL=http://localhost:8000 \
  IDT4_URL=http://localhost:8401 \
  go run ./cmd/cdt5 &
PIDS+=($!)

echo "Started 5 lower cDT services"
sleep 3

# ── Upper cDT Layer ────────────────────────────────────────────────────────────
PORT=8601 ARROWHEAD_URL=http://localhost:8000 \
  CDT1_URL=http://localhost:8501 CDT3_URL=http://localhost:8503 \
  CDT4_URL=http://localhost:8504 CDT5_URL=http://localhost:8505 \
  go run ./cmd/cdta &
PIDS+=($!)

PORT=8602 ARROWHEAD_URL=http://localhost:8000 \
  CDT2_URL=http://localhost:8502 CDT3_URL=http://localhost:8503 \
  go run ./cmd/cdtb &
PIDS+=($!)

echo "Started 2 upper cDT services"
sleep 2

# ── Scenario Runner ────────────────────────────────────────────────────────────
PORT=8700 ARROWHEAD_URL=http://localhost:8000 \
  IDT1A_URL=http://localhost:8101 IDT1B_URL=http://localhost:8102 \
  IDT2A_URL=http://localhost:8201 IDT2B_URL=http://localhost:8202 \
  IDT3A_URL=http://localhost:8301 IDT3B_URL=http://localhost:8302 \
  IDT4_URL=http://localhost:8401 \
  CDT1_URL=http://localhost:8501 CDT2_URL=http://localhost:8502 \
  CDT3_URL=http://localhost:8503 CDT4_URL=http://localhost:8504 \
  CDT5_URL=http://localhost:8505 \
  CDTA_URL=http://localhost:8601 CDTB_URL=http://localhost:8602 \
  go run ./cmd/scenario &
PIDS+=($!)
echo "Started scenario runner on :8700"

echo ""
echo "========================================"
echo "All backend services running!"
echo "Arrowhead Core:     http://localhost:8000"
echo "Inspection Robot A: http://localhost:8101"
echo "Inspection Robot B: http://localhost:8102"
echo "Gas Sensing A:      http://localhost:8201"
echo "Gas Sensing B:      http://localhost:8202"
echo "LHD Vehicle A:      http://localhost:8301"
echo "LHD Vehicle B:      http://localhost:8302"
echo "Tele-Remote:        http://localhost:8401"
echo "cDT1 Mapping:       http://localhost:8501"
echo "cDT2 Gas Monitor:   http://localhost:8502"
echo "cDT3 Hazard:        http://localhost:8503"
echo "cDT4 Clearance:     http://localhost:8504"
echo "cDT5 Intervention:  http://localhost:8505"
echo "cDTa Mission:       http://localhost:8601"
echo "cDTb Safe Access:   http://localhost:8602"
echo "Scenario Runner:    http://localhost:8700"
echo "========================================"

# ── Frontend ───────────────────────────────────────────────────────────────────
echo "Starting frontend..."
cd "$FRONTEND"
npm install --silent
npm run dev &
PIDS+=($!)

echo "Frontend:           http://localhost:3000"
echo ""
echo "Press Ctrl+C to stop all services."

wait

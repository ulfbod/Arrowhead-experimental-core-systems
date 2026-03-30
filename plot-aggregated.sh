#!/usr/bin/env bash
# plot-aggregated.sh  –  Aggregate experiment CSVs and plot with gnuplot.
#
# Usage:
#   ./plot-aggregated.sh                        # default: results/ → docs/figures/
#   ./plot-aggregated.sh --input-dir results/
#   ./plot-aggregated.sh --output-dir /tmp/plots
#   ./plot-aggregated.sh --scenario tradeoff
#   ./plot-aggregated.sh --help

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
INPUT_DIR="results"
OUTPUT_DIR="docs/figures"
SCENARIO="all"   # tradeoff | degradation | all

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --input-dir)  INPUT_DIR="$2";  shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --scenario)   SCENARIO="$2";   shift 2 ;;
    --help|-h)
      grep '^#' "$0" | sed 's/^# \{0,2\}//'
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info() { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
die()  { echo -e "${RED}[ERR ]${NC}  $*" >&2; exit 1; }

# ── Prerequisites ─────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INPUT_DIR="$SCRIPT_DIR/$INPUT_DIR"
OUTPUT_DIR="$SCRIPT_DIR/$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

command -v gnuplot >/dev/null 2>&1 || die "gnuplot not found.  Install: sudo apt install gnuplot"

if [[ -x "$SCRIPT_DIR/.venv/bin/python" ]]; then
  PYTHON="$SCRIPT_DIR/.venv/bin/python"
else
  command -v python3 >/dev/null 2>&1 || die "python3 not found"
  PYTHON="python3"
fi

echo ""
info "Input    : $INPUT_DIR"
info "Output   : $OUTPUT_DIR"
info "Scenario : $SCENARIO"
echo ""

# ── Step 1: Python aggregation ────────────────────────────────────────────────
info "Aggregating CSVs…"

AGG_INPUT="$INPUT_DIR" AGG_OUTPUT="$OUTPUT_DIR" AGG_SCENARIO="$SCENARIO" "$PYTHON" - <<'PYEOF'
import csv, sys, os
import numpy as np
from collections import defaultdict
from pathlib import Path

inp = os.environ["AGG_INPUT"]
out = os.environ["AGG_OUTPUT"]
scenario = os.environ["AGG_SCENARIO"]

def summarize(input_csv, group_col, value_col, output_csv,
              filter_col=None, filter_val=None):
    p = Path(input_csv)
    if not p.exists():
        print(f"  [skip] {p} not found", flush=True)
        return False
    rows = list(csv.DictReader(open(p)))
    if filter_col:
        rows = [r for r in rows if r[filter_col] == filter_val]
    if not rows:
        print(f"  [skip] {p}: no matching rows", flush=True)
        return False
    buckets = defaultdict(list)
    for r in rows:
        buckets[float(r[group_col])].append(float(r[value_col]))
    Path(output_csv).parent.mkdir(parents=True, exist_ok=True)
    with open(output_csv, "w") as f:
        f.write(f"{group_col},median,p10,p90\n")
        for x in sorted(buckets):
            v = np.array(buckets[x])
            f.write(f"{x},{np.median(v):.6f},{np.percentile(v,10):.6f},{np.percentile(v,90):.6f}\n")
    print(f"  written: {output_csv}", flush=True)
    return True

if scenario in ("tradeoff", "all"):
    summarize(f"{inp}/tradeoff/aggregated.csv",
              "alpha", "selected_utility",
              f"{out}/tradeoff_summary.csv")

if scenario in ("degradation", "all"):
    summarize(f"{inp}/degradation/aggregated.csv",
              "t", "utility",
              f"{out}/degradation_summary_qos.csv",
              filter_col="method", filter_val="qos_aware")
    summarize(f"{inp}/degradation/aggregated.csv",
              "t", "utility",
              f"{out}/degradation_summary_avail.csv",
              filter_col="method", filter_val="availability_based")

    # Advantage CSV: per-timestep delta (qos_aware - availability_based), paired by run
    p = Path(f"{inp}/degradation/aggregated.csv")
    if p.exists():
        rows = list(csv.DictReader(open(p)))
        qos   = defaultdict(dict)
        avail = defaultdict(dict)
        for r in rows:
            t   = int(r["t"])
            run = int(r["run"])
            if r["method"] == "qos_aware":
                qos[t][run] = float(r["utility"])
            elif r["method"] == "availability_based":
                avail[t][run] = float(r["utility"])
        adv_path = f"{out}/degradation_advantage_summary.csv"
        Path(adv_path).parent.mkdir(parents=True, exist_ok=True)
        with open(adv_path, "w") as f:
            f.write("t,median,p10,p90\n")
            for t in sorted(qos):
                common = sorted(set(qos[t]) & set(avail[t]))
                if not common:
                    continue
                deltas = np.array([qos[t][r] - avail[t][r] for r in common])
                f.write(f"{t},{np.median(deltas):.6f},{np.percentile(deltas,10):.6f},{np.percentile(deltas,90):.6f}\n")
        print(f"  written: {adv_path}", flush=True)
PYEOF
ok "Aggregation done."
echo ""

# ── Helper: write a gnuplot script to a temp file, run it, remove it ──────────
run_gnuplot() {
  local name="$1"
  local script="$2"
  local gp_file
  gp_file="$(mktemp /tmp/gnuplot_XXXXXX.gp)"
  printf '%s\n' "$script" > "$gp_file"
  if gnuplot "$gp_file" 2>&1; then
    ok "  $name"
  else
    warn "  $name  (gnuplot reported an error)"
  fi
  rm -f "$gp_file"
}

# ── Step 2: Trade-off plots ───────────────────────────────────────────────────
if [[ "$SCENARIO" == "tradeoff" || "$SCENARIO" == "all" ]]; then
  SUMMARY="$OUTPUT_DIR/tradeoff_summary.csv"
  if [[ -f "$SUMMARY" ]]; then
    info "Plotting trade-off…"
    run_gnuplot \
      "$OUTPUT_DIR/tradeoff_utility_vs_weight_gnuplot.{png,pdf}" \
      "set datafile separator \",\"
set grid lc rgb \"#cccccc\" lw 1
set border lw 1.5
set key top left
set xlabel \"Accuracy weight (alpha)\"
set ylabel \"Utility of selected provider\"
set title \"QoS Trade-off: Utility vs. Accuracy Weight\"
set xrange [0:1]
set yrange [0:1]
set terminal pngcairo size 800,500
set output \"$OUTPUT_DIR/tradeoff_utility_vs_weight_gnuplot.png\"
plot \"$SUMMARY\" skip 1 using 1:3:4 with filledcurves lc rgb \"#2563eb\" fs transparent solid 0.18 notitle, \\
     \"$SUMMARY\" skip 1 using 1:2   with linespoints  lc rgb \"#2563eb\" lw 2 pt 7 ps 0.8 title \"Median (p10-p90 band)\"
set terminal pdfcairo size 6,4
set output \"$OUTPUT_DIR/tradeoff_utility_vs_weight_gnuplot.pdf\"
replot
set output"
  else
    warn "Skipping trade-off: $SUMMARY not found"
  fi
fi

# ── Step 3: Degradation plots ─────────────────────────────────────────────────
if [[ "$SCENARIO" == "degradation" || "$SCENARIO" == "all" ]]; then
  QOS_CSV="$OUTPUT_DIR/degradation_summary_qos.csv"
  AVAIL_CSV="$OUTPUT_DIR/degradation_summary_avail.csv"
  ADV_CSV="$OUTPUT_DIR/degradation_advantage_summary.csv"

  if [[ -f "$QOS_CSV" && -f "$AVAIL_CSV" ]]; then
    info "Plotting degradation utility over time…"
    run_gnuplot \
      "$OUTPUT_DIR/degradation_utility_over_time_gnuplot.{png,pdf}" \
      "set datafile separator \",\"
set grid lc rgb \"#cccccc\" lw 1
set border lw 1.5
set key bottom left
set xlabel \"Simulation time (s)\"
set ylabel \"Utility\"
set title \"Controlled Degradation: QoS-Aware vs. Availability-Based\"
set xrange [0:60]
set yrange [0:1.05]
set terminal pngcairo size 900,500
set output \"$OUTPUT_DIR/degradation_utility_over_time_gnuplot.png\"
plot \"$QOS_CSV\"   skip 1 using 1:3:4 with filledcurves lc rgb \"#2563eb\" fs transparent solid 0.18 notitle, \\
     \"$QOS_CSV\"   skip 1 using 1:2   with lines        lc rgb \"#2563eb\" lw 2 title \"QoS-aware (median)\", \\
     \"$AVAIL_CSV\" skip 1 using 1:3:4 with filledcurves lc rgb \"#dc2626\" fs transparent solid 0.18 notitle, \\
     \"$AVAIL_CSV\" skip 1 using 1:2   with lines        lc rgb \"#dc2626\" lw 2 title \"Availability-based (median)\"
set terminal pdfcairo size 7,4
set output \"$OUTPUT_DIR/degradation_utility_over_time_gnuplot.pdf\"
replot
set output"

    if [[ -f "$ADV_CSV" ]]; then
      info "Plotting utility advantage…"
      run_gnuplot \
        "$OUTPUT_DIR/degradation_utility_advantage_gnuplot.{png,pdf}" \
        "set datafile separator \",\"
set grid lc rgb \"#cccccc\" lw 1
set border lw 1.5
set key top right
set xlabel \"Simulation time (s)\"
set ylabel \"Utility advantage (QoS-aware minus baseline)\"
set title \"QoS-Aware Utility Advantage Over Availability-Based Selection\"
set xrange [0:60]
set terminal pngcairo size 800,450
set output \"$OUTPUT_DIR/degradation_utility_advantage_gnuplot.png\"
plot \"$ADV_CSV\" skip 1 using 1:3:4 with filledcurves lc rgb \"#2563eb\" fs transparent solid 0.18 notitle, \\
     \"$ADV_CSV\" skip 1 using 1:2   with lines        lc rgb \"#2563eb\" lw 2 title \"Median advantage\", \\
     0                               with lines        lc rgb \"black\"   lw 1 dt 2  title \"No advantage\"
set terminal pdfcairo size 7,4
set output \"$OUTPUT_DIR/degradation_utility_advantage_gnuplot.pdf\"
replot
set output"
    fi
  else
    warn "Skipping degradation: summary CSVs not found"
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
info "Output files:"
for f in \
  "$OUTPUT_DIR/tradeoff_summary.csv" \
  "$OUTPUT_DIR/tradeoff_utility_vs_weight_gnuplot.png" \
  "$OUTPUT_DIR/tradeoff_utility_vs_weight_gnuplot.pdf" \
  "$OUTPUT_DIR/degradation_summary_qos.csv" \
  "$OUTPUT_DIR/degradation_summary_avail.csv" \
  "$OUTPUT_DIR/degradation_advantage_summary.csv" \
  "$OUTPUT_DIR/degradation_utility_over_time_gnuplot.png" \
  "$OUTPUT_DIR/degradation_utility_over_time_gnuplot.pdf" \
  "$OUTPUT_DIR/degradation_utility_advantage_gnuplot.png" \
  "$OUTPUT_DIR/degradation_utility_advantage_gnuplot.pdf"; do
  [[ -f "$f" ]] && ok "  $f" || warn "  $f  (not generated)"
done
echo ""

#!/usr/bin/env python3
"""
Generate publication-quality plots from experiment results.

Reads CSV files produced by experiments.py (and the Go failover benchmark) and
writes PNG + PDF figures suitable for embedding in a paper or README.

Output files (in --output-dir):
  tradeoff_utility_vs_weight.{png,pdf}
  tradeoff_qos_metrics_vs_weight.{png,pdf}
  degradation_utility_over_time.{png,pdf}
  degradation_utility_advantage.{png,pdf}
  failover_delay_vs_network_delay.{png,pdf}   (from Go benchmark CSV)

Usage:
  python scripts/plot.py --input-dir results/ --output-dir docs/figures/
  python scripts/plot.py --scenario tradeoff --input-dir results/
"""
import argparse
import csv
import sys
from collections import defaultdict
from pathlib import Path

try:
    import numpy as np
    import matplotlib
    matplotlib.use("Agg")   # non-interactive backend; no display required
    import matplotlib.pyplot as plt
    import matplotlib.ticker as ticker
except ImportError as exc:
    print(f"ERROR: {exc}\n  pip install numpy matplotlib", file=sys.stderr)
    sys.exit(1)

# ── House style ───────────────────────────────────────────────────────────────

COLORS = {
    "primary":            "#2563eb",   # blue
    "fallback":           "#dc2626",   # red
    "qos_aware":          "#2563eb",   # blue
    "availability_based": "#dc2626",   # red
    "local":              "#16a34a",   # green
    "central":            "#9333ea",   # purple
    "neutral":            "#64748b",   # slate
}
BAND_ALPHA  = 0.18
LINE_WIDTH  = 2.0
MARKER_SIZE = 4
FIG_DPI     = 150
SIM_DURATION_S = 60   # must match experiments.py constant


def _style(ax):
    """Apply minimal publication style to axes."""
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.grid(True, linestyle="--", linewidth=0.5, alpha=0.45, color="gray")
    ax.tick_params(labelsize=9)


def _save(fig, directory: Path, name: str) -> None:
    directory.mkdir(parents=True, exist_ok=True)
    for ext in ("png", "pdf"):
        out = directory / f"{name}.{ext}"
        fig.savefig(out, dpi=FIG_DPI, bbox_inches="tight")
        print(f"  saved → {out}")
    plt.close(fig)


def _read_csv(path: Path) -> list:
    with open(path, newline="") as f:
        return list(csv.DictReader(f))


# ── Statistical helpers ───────────────────────────────────────────────────────

def _band(values_by_x: dict, x_keys: list) -> tuple:
    """Return (x_arr, median_arr, p10_arr, p90_arr) from a {key → [values]} dict."""
    medians, p10s, p90s = [], [], []
    for x in x_keys:
        v = np.array(values_by_x[x], dtype=float)
        medians.append(float(np.median(v)))
        p10s.append(float(np.percentile(v, 10)))
        p90s.append(float(np.percentile(v, 90)))
    return (
        np.array(x_keys, dtype=float),
        np.array(medians), np.array(p10s), np.array(p90s),
    )


# ── Plot 1+2: QoS Trade-off Analysis ─────────────────────────────────────────

def plot_tradeoff(input_dir: Path, output_dir: Path) -> None:
    agg = input_dir / "tradeoff" / "aggregated.csv"
    if not agg.exists():
        print(f"  [skip] {agg} not found")
        return

    rows = _read_csv(agg)

    # Group data by alpha value
    utility_by_a: dict = defaultdict(list)
    acc_sel_by_a: dict = defaultdict(list)
    lat_sel_by_a: dict = defaultdict(list)
    rel_sel_by_a: dict = defaultdict(list)

    for r in rows:
        alpha = round(float(r["alpha"]), 4)
        sel   = r["selected"]
        utility_by_a[alpha].append(float(r["selected_utility"]))
        acc_sel_by_a[alpha].append(float(r[f"prov_{'a' if sel == 'idt2a' else 'b'}_accuracy"]))
        lat_sel_by_a[alpha].append(float(r[f"prov_{'a' if sel == 'idt2a' else 'b'}_latency_ms"]))
        rel_sel_by_a[alpha].append(float(r[f"prov_{'a' if sel == 'idt2a' else 'b'}_reliability"]))

    alphas = sorted(utility_by_a.keys())

    # ── Figure 1: utility vs alpha ────────────────────────────────────────────
    fig, ax = plt.subplots(figsize=(6, 4))
    x, med, p10, p90 = _band(utility_by_a, alphas)
    ax.plot(x, med, color=COLORS["primary"], linewidth=LINE_WIDTH,
            label="Selected utility (median)")
    ax.fill_between(x, p10, p90, color=COLORS["primary"], alpha=BAND_ALPHA,
                    label="10th–90th percentile band")
    ax.set_xlabel(r"Accuracy weight $w_\mathrm{acc}$", fontsize=11)
    ax.set_ylabel("Utility of selected provider", fontsize=11)
    ax.set_title("QoS Trade-off: Utility vs. Accuracy Weight", fontsize=12)
    ax.set_xlim(0, 1)
    ax.set_ylim(0, 1)
    ax.legend(fontsize=9)
    _style(ax)
    fig.tight_layout()
    _save(fig, output_dir, "tradeoff_utility_vs_weight")

    # ── Figure 2: QoS metrics vs alpha (3 sub-panels) ────────────────────────
    fig, axes = plt.subplots(1, 3, figsize=(13, 4), sharey=False)
    panels = [
        (acc_sel_by_a, "Accuracy", COLORS["primary"]),
        (lat_sel_by_a, "Latency (ms)", COLORS["fallback"]),
        (rel_sel_by_a, "Reliability", COLORS["local"]),
    ]
    for ax, (data, ylabel, color) in zip(axes, panels):
        x, med, p10, p90 = _band(data, alphas)
        ax.plot(x, med, color=color, linewidth=LINE_WIDTH)
        ax.fill_between(x, p10, p90, color=color, alpha=BAND_ALPHA)
        ax.set_xlabel(r"Accuracy weight $w_\mathrm{acc}$", fontsize=10)
        ax.set_ylabel(ylabel, fontsize=10)
        ax.set_title(ylabel, fontsize=11)
        ax.set_xlim(0, 1)
        _style(ax)

    fig.suptitle(
        "QoS Trade-off: Attributes of Selected Provider vs. Accuracy Weight",
        fontsize=12, y=1.01,
    )
    fig.tight_layout()
    _save(fig, output_dir, "tradeoff_qos_metrics_vs_weight")

    print("  [tradeoff] done")


# ── Plot 3+4: Controlled Degradation ─────────────────────────────────────────

def plot_degradation(input_dir: Path, output_dir: Path) -> None:
    agg = input_dir / "degradation" / "aggregated.csv"
    if not agg.exists():
        print(f"  [skip] {agg} not found")
        return

    rows = _read_csv(agg)

    methods = ["qos_aware", "availability_based"]
    util_by_mt: dict  = {m: defaultdict(list) for m in methods}
    fo_count_mt: dict = {m: defaultdict(int)  for m in methods}

    for r in rows:
        m = r["method"]
        if m not in util_by_mt:
            continue
        t = int(r["t"])
        util_by_mt[m][t].append(float(r["utility"]))
        if int(r["failover_event"]) == 1:
            fo_count_mt[m][t] += 1

    ts = sorted(util_by_mt["qos_aware"].keys())

    # ── Figure 3: utility over time, both methods ─────────────────────────────
    fig, ax = plt.subplots(figsize=(8, 4.5))

    labels = {
        "qos_aware":          "QoS-aware (proposed)",
        "availability_based": "Availability-based (baseline)",
    }
    for m in methods:
        if not util_by_mt[m]:
            continue
        color = COLORS.get(m, COLORS["neutral"])
        x, med, p10, p90 = _band(util_by_mt[m], ts)
        ax.plot(x, med, color=color, linewidth=LINE_WIDTH, label=labels.get(m, m))
        ax.fill_between(x, p10, p90, color=color, alpha=BAND_ALPHA)

    # Shade the typical QoS-aware failover window across all runs
    fo_ts = sorted(t for t, c in fo_count_mt["qos_aware"].items() if c > 0)
    if fo_ts:
        ax.axvspan(min(fo_ts), max(fo_ts) + 1, alpha=0.08, color="orange",
                   label="QoS-aware failover window (across runs)")

    ax.set_xlabel("Simulation time (s)", fontsize=11)
    ax.set_ylabel("Utility", fontsize=11)
    ax.set_title(
        "Controlled Degradation: QoS-Aware vs. Availability-Based Selection",
        fontsize=12,
    )
    ax.legend(fontsize=9, loc="lower left")
    ax.set_xlim(0, SIM_DURATION_S)
    ax.set_ylim(0, 1.05)
    _style(ax)
    fig.tight_layout()
    _save(fig, output_dir, "degradation_utility_over_time")

    # ── Figure 4: utility advantage (QoS-aware − baseline) per timestep ──────
    # Pair runs by run index: sort each method's values so they align
    util_q  = {t: sorted(util_by_mt["qos_aware"][t])          for t in ts}
    util_ab = {t: sorted(util_by_mt["availability_based"].get(t, [0.0])) for t in ts}

    delta_med, delta_p10, delta_p90 = [], [], []
    for t in ts:
        q = np.array(util_q[t])
        a = np.array(util_ab[t])
        n = min(len(q), len(a))
        if n == 0:
            delta_med.append(0.0); delta_p10.append(0.0); delta_p90.append(0.0)
            continue
        d = q[:n] - a[:n]
        delta_med.append(float(np.median(d)))
        delta_p10.append(float(np.percentile(d, 10)))
        delta_p90.append(float(np.percentile(d, 90)))

    ts_arr     = np.array(ts, dtype=float)
    delta_med  = np.array(delta_med)
    delta_p10  = np.array(delta_p10)
    delta_p90  = np.array(delta_p90)

    fig, ax = plt.subplots(figsize=(7, 4))
    ax.plot(ts_arr, delta_med, color=COLORS["qos_aware"], linewidth=LINE_WIDTH,
            label="Median advantage")
    ax.fill_between(ts_arr, delta_p10, delta_p90,
                    color=COLORS["qos_aware"], alpha=BAND_ALPHA,
                    label="10th–90th percentile band")
    ax.axhline(0, color="black", linewidth=0.8, linestyle="--", label="No advantage")
    ax.set_xlabel("Simulation time (s)", fontsize=11)
    ax.set_ylabel("Utility advantage (QoS-aware − baseline)", fontsize=11)
    ax.set_title("QoS-Aware Utility Advantage Over Availability-Based Selection", fontsize=12)
    ax.legend(fontsize=9)
    ax.set_xlim(0, SIM_DURATION_S)
    _style(ax)
    fig.tight_layout()
    _save(fig, output_dir, "degradation_utility_advantage")

    print("  [degradation] done")


# ── Plot 5: Failover delay benchmark (existing Go CSV) ───────────────────────

def plot_failover(input_dir: Path, output_dir: Path) -> None:
    """Plot local vs. centralised failover decision delay from the Go benchmark."""
    csv_path = input_dir / "failover_delay_vs_network_delay.csv"
    if not csv_path.exists():
        print(f"  [skip] {csv_path} not found")
        return

    rows = _read_csv(csv_path)
    if not rows or "local_avg_ms" not in rows[0]:
        print(f"  [skip] {csv_path} has unexpected format")
        return

    try:
        delays = np.array([float(r["network_delay_ms"]) for r in rows])
        l_avg  = np.array([float(r["local_avg_ms"])     for r in rows])
        l_p10  = np.array([float(r["local_p10_ms"])     for r in rows])
        l_p90  = np.array([float(r["local_p90_ms"])     for r in rows])
        c_avg  = np.array([float(r["central_avg_ms"])   for r in rows])
        c_p10  = np.array([float(r["central_p10_ms"])   for r in rows])
        c_p90  = np.array([float(r["central_p90_ms"])   for r in rows])
    except (KeyError, ValueError) as exc:
        print(f"  [skip] failover CSV format error: {exc}")
        return

    fig, ax = plt.subplots(figsize=(7, 4))
    ax.plot(delays, l_avg, color=COLORS["local"],   linewidth=LINE_WIDTH,
            marker="o", markersize=MARKER_SIZE, label="Local failover (avg)")
    ax.fill_between(delays, l_p10, l_p90,
                    color=COLORS["local"], alpha=BAND_ALPHA, label="Local p10–p90")
    ax.plot(delays, c_avg, color=COLORS["central"], linewidth=LINE_WIDTH,
            marker="s", markersize=MARKER_SIZE, label="Centralised orchestration (avg)")
    ax.fill_between(delays, c_p10, c_p90,
                    color=COLORS["central"], alpha=BAND_ALPHA, label="Centralised p10–p90")
    ax.set_xlabel("Simulated one-way network latency (ms)", fontsize=11)
    ax.set_ylabel("Failover decision delay (ms)", fontsize=11)
    ax.set_title("Failover Decision Delay vs. Network Latency", fontsize=12)
    ax.legend(fontsize=9)
    _style(ax)
    fig.tight_layout()
    _save(fig, output_dir, "failover_delay_vs_network_delay")

    print("  [failover] done")


# ── CLI ───────────────────────────────────────────────────────────────────────

def main() -> None:
    p = argparse.ArgumentParser(
        description="Generate publication-quality plots from experiment results.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    p.add_argument("--input-dir",  type=Path, default=Path("results"),
                   help="Root directory that contains aggregated CSVs")
    p.add_argument("--output-dir", type=Path, default=Path("docs/figures"),
                   help="Directory to write PNG and PDF plots")
    p.add_argument("--scenario",
                   choices=["tradeoff", "degradation", "failover", "all"],
                   default="all",
                   help="Which set of plots to generate")
    args = p.parse_args()

    print(f"\n{'='*62}")
    print(f"  Scenario   : {args.scenario}")
    print(f"  Input dir  : {args.input_dir.resolve()}")
    print(f"  Output dir : {args.output_dir.resolve()}")
    print(f"{'='*62}\n")

    if args.scenario in ("tradeoff", "all"):
        plot_tradeoff(args.input_dir, args.output_dir)
    if args.scenario in ("degradation", "all"):
        plot_degradation(args.input_dir, args.output_dir)
    if args.scenario in ("failover", "all"):
        plot_failover(args.input_dir, args.output_dir)

    print(f"\nAll figures written to: {args.output_dir.resolve()}\n")


if __name__ == "__main__":
    main()

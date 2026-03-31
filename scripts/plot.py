#!/usr/bin/env python3
"""
Generate publication-quality plots from experiment results.

Reads CSV files produced by experiments.py (and the Go failover benchmark) and
writes PNG + PDF figures suitable for embedding in a paper or README.

Output files (in --output-dir/<eval-scenario>/):
  tradeoff_provider_utilities.{png,pdf}     ← both provider utility curves + crossover
  tradeoff_qos_metrics_vs_weight.{png,pdf}  ← 3-panel: accuracy / latency / reliability
  degradation_combined.{png,pdf}            ← 2-panel: utility + advantage (stacked)
  failover_delay_vs_network_delay.{png,pdf} ← Go benchmark (if CSV present)

Usage:
  python scripts/plot.py --input-dir results/ --output-dir docs/figures/
  python scripts/plot.py --eval-scenario improved01 --scenario degradation
"""
import argparse
import csv
import sys
from collections import defaultdict
from pathlib import Path

try:
    import numpy as np
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt
    import matplotlib.patches as mpatches
    import matplotlib.lines  as mlines
except ImportError as exc:
    print(f"ERROR: {exc}\n  pip install numpy matplotlib", file=sys.stderr)
    sys.exit(1)

# ── Publication style ─────────────────────────────────────────────────────────

COLOR_PROPOSED   = "#1d4ed8"   # dark blue  — proposed QoS-aware
COLOR_BASELINE   = "#b91c1c"   # dark red   — availability-based baseline
COLOR_PROVIDER_A = "#1d4ed8"   # blue       — quality / accurate provider
COLOR_PROVIDER_B = "#b91c1c"   # red        — fast / noisy provider
COLOR_DEGRADE    = "#d1d5db"   # light gray — degradation window shading
COLOR_LOCAL      = "#15803d"   # green
COLOR_CENTRAL    = "#7c3aed"   # purple

LW_MAIN          = 2.2    # main median curve
LW_BAND          = 0.9    # p10/p90 dashed lines
LW_ZERO          = 0.9    # zero-reference line
BAND_FILL_ALPHA  = 0.12
MARKER_EVERY     = 8      # place a marker every N x-values on median curves
FIG_DPI          = 150

# Minimal per-eval-scenario plot configuration (mirrors experiments.py SCENARIO_CONFIGS).
# Only the values needed by the plot layer are stored here.
EVAL_SCENARIO_PLOT_CFG: dict = {
    "basic": {
        "label":          "Basic",
        "sim_duration_s": 120,
        "episode_count":  2,
    },
    "improved01": {
        "label":          "Improved (enhanced discriminability)",
        "sim_duration_s": 200,
        "episode_count":  3,
    },
}


# ── Helpers ───────────────────────────────────────────────────────────────────

def _style(ax, ylim=None):
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.grid(True, linestyle=":", linewidth=0.5, alpha=0.55, color="#9ca3af")
    ax.tick_params(labelsize=9)
    if ylim is not None:
        ax.set_ylim(*ylim)


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
    """Return (x, median, p10, p90) arrays from a {x → [values]} dict."""
    medians, p10s, p90s = [], [], []
    for x in x_keys:
        v = np.array(values_by_x[x], dtype=float)
        medians.append(np.median(v))
        p10s.append(np.percentile(v, 10))
        p90s.append(np.percentile(v, 90))
    return (
        np.array(x_keys, dtype=float),
        np.array(medians), np.array(p10s), np.array(p90s),
    )


def _plot_with_band(ax, x, med, p10, p90, color, linestyle="-", label_med=None,
                    label_band=None, marker=None):
    """Plot a median curve with explicit dashed p10/p90 lines and a light fill."""
    kw = dict(color=color, linewidth=LW_MAIN, linestyle=linestyle, zorder=3)
    if marker:
        step = max(1, len(x) // MARKER_EVERY)
        kw.update(marker=marker, markevery=step, markersize=5, markeredgewidth=0.8)
    ax.plot(x, med, label=label_med, **kw)

    band_kw = dict(color=color, linewidth=LW_BAND, linestyle="--", alpha=0.70, zorder=2)
    ax.plot(x, p10, **band_kw)
    ax.plot(x, p90, label=label_band, **band_kw)

    ax.fill_between(x, p10, p90, color=color, alpha=BAND_FILL_ALPHA, zorder=1)


# ── Degradation window shading ────────────────────────────────────────────────

def _shade_degrade_windows(ax, rows: list, alpha_fill: float = 0.18) -> None:
    """Shade the median degradation windows for all episodes found in the CSV.

    Iterates episode columns 1 .. MAX_EPISODES, skipping absent or empty slots.
    """
    if not rows:
        return
    for ep_n in range(1, 4):   # up to 3 episodes
        key_onset = f"onset{ep_n}_s"
        key_fail  = f"fail{ep_n}_s"
        if key_onset not in rows[0]:
            break  # column absent entirely
        seen = set()
        onsets, fails = [], []
        for r in rows:
            rid = r["run"]
            if rid in seen:
                continue
            seen.add(rid)
            val = r.get(key_onset, "")
            if val == "":
                continue   # episode not used in this scenario
            try:
                onsets.append(float(val))
                fails.append(float(r[key_fail]))
            except (ValueError, KeyError):
                continue
        if not onsets:
            continue
        med_onset = float(np.median(onsets))
        med_fail  = float(np.median(fails))
        ax.axvspan(med_onset, med_fail, color=COLOR_DEGRADE, alpha=alpha_fill, zorder=0)


# ── Plot 1: QoS Trade-off — provider utility crossover ───────────────────────

def plot_tradeoff(input_dir: Path, output_dir: Path, eval_scenario: str) -> None:
    agg = input_dir / "tradeoff" / "aggregated.csv"
    if not agg.exists():
        print(f"  [skip] {agg} not found")
        return

    rows   = _read_csv(agg)
    n_runs = len({r["run"] for r in rows})
    pcfg   = EVAL_SCENARIO_PLOT_CFG.get(eval_scenario, EVAL_SCENARIO_PLOT_CFG["basic"])
    slabel = pcfg["label"]

    util_a_by_alpha: dict  = defaultdict(list)
    util_b_by_alpha: dict  = defaultdict(list)
    sel_util_by_alpha: dict = defaultdict(list)

    for r in rows:
        alpha = round(float(r["alpha"]), 4)
        util_a_by_alpha[alpha].append(float(r["utility_a"]))
        util_b_by_alpha[alpha].append(float(r["utility_b"]))
        sel_util_by_alpha[alpha].append(float(r["selected_utility"]))

    alphas = sorted(util_a_by_alpha.keys())

    # ── Figure 1A: both provider utilities with crossover ────────────────────
    fig, ax = plt.subplots(figsize=(7, 4.5))

    x, med_a, p10_a, p90_a = _band(util_a_by_alpha, alphas)
    x, med_b, p10_b, p90_b = _band(util_b_by_alpha, alphas)

    _plot_with_band(ax, x, med_a, p10_a, p90_a,
                    color=COLOR_PROVIDER_A, linestyle="-", marker="o",
                    label_med="Provider A — quality sensor (median)",
                    label_band="Provider A (p10 / p90)")
    _plot_with_band(ax, x, med_b, p10_b, p90_b,
                    color=COLOR_PROVIDER_B, linestyle="--", marker="s",
                    label_med="Provider B — fast sensor (median)",
                    label_band="Provider B (p10 / p90)")

    # Mark crossover points
    crossover_alphas = [alphas[i] for i in range(len(alphas) - 1)
                        if (med_a[i] < med_b[i]) != (med_a[i+1] < med_b[i+1])]
    for ca in crossover_alphas:
        ax.axvline(ca, color="#6b7280", linewidth=1.0, linestyle=":", zorder=2)
        ax.text(ca + 0.02, 0.08, f"crossover\nα ≈ {ca:.2f}",
                fontsize=7.5, color="#374151", va="bottom")

    ax.set_xlabel(
        r"Accuracy weight $w_\mathrm{acc}$  "
        r"(latency and reliability share $1-w_\mathrm{acc}$ equally)",
        fontsize=10,
    )
    ax.set_ylabel("Provider utility", fontsize=11)
    ax.set_title(
        f"QoS Trade-off: Provider Utility vs. Accuracy Weight  [{slabel}]\n"
        f"(shaded: 10th–90th percentile across {n_runs} runs)",
        fontsize=11,
    )
    ax.set_xlim(0, 1)
    ax.set_ylim(0, 1.05)
    ax.legend(fontsize=8.5, loc="center right", framealpha=0.9)
    ax.annotate("Shaded region: p10–p90 across runs",
                xy=(0.02, 0.03), xycoords="axes fraction",
                fontsize=7.5, color="#6b7280")
    _style(ax)
    fig.tight_layout()
    _save(fig, output_dir, "tradeoff_provider_utilities")

    # ── Figure 1B: QoS metrics of selected provider (3 sub-panels) ───────────
    acc_sel: dict = defaultdict(list)
    lat_sel: dict = defaultdict(list)
    rel_sel: dict = defaultdict(list)

    for r in rows:
        alpha = round(float(r["alpha"]), 4)
        sel   = r["selected"]
        pref  = "a" if sel == "idt2a" else "b"
        acc_sel[alpha].append(float(r[f"prov_{pref}_accuracy"]))
        lat_sel[alpha].append(float(r[f"prov_{pref}_latency_ms"]))
        rel_sel[alpha].append(float(r[f"prov_{pref}_reliability"]))

    fig, axes = plt.subplots(1, 3, figsize=(13, 4), sharey=False)
    panels = [
        (acc_sel, "Accuracy of selected provider",     "#1d4ed8"),
        (lat_sel, "Latency of selected provider (ms)", "#b91c1c"),
        (rel_sel, "Reliability of selected provider",  "#15803d"),
    ]
    for ax, (data, ylabel, color) in zip(axes, panels):
        x, med, p10, p90 = _band(data, alphas)
        ax.plot(x, med, color=color, linewidth=LW_MAIN, linestyle="-",  label="Median")
        ax.plot(x, p10, color=color, linewidth=LW_BAND, linestyle="--", alpha=0.70,
                label="p10 / p90")
        ax.plot(x, p90, color=color, linewidth=LW_BAND, linestyle="--", alpha=0.70)
        ax.fill_between(x, p10, p90, color=color, alpha=BAND_FILL_ALPHA)
        ax.set_xlabel(r"$w_\mathrm{acc}$", fontsize=10)
        ax.set_ylabel(ylabel, fontsize=10)
        ax.set_title(ylabel, fontsize=10)
        ax.set_xlim(0, 1)
        ax.legend(fontsize=8)
        _style(ax)

    fig.suptitle(
        f"QoS Trade-off: Attributes of Selected Provider vs. Accuracy Weight  [{slabel}]\n"
        "(dashed lines: 10th and 90th percentile across runs)",
        fontsize=11, y=1.03,
    )
    fig.tight_layout()
    _save(fig, output_dir, "tradeoff_qos_metrics_vs_weight")

    print("  [tradeoff] done")


# ── Plot 2: Controlled Degradation — 2-panel stacked ─────────────────────────

def plot_degradation(input_dir: Path, output_dir: Path, eval_scenario: str) -> None:
    agg = input_dir / "degradation" / "aggregated.csv"
    if not agg.exists():
        print(f"  [skip] {agg} not found")
        return

    rows   = _read_csv(agg)
    n_runs = len({r["run"] for r in rows if r["method"] == "qos_aware"})
    pcfg   = EVAL_SCENARIO_PLOT_CFG.get(eval_scenario, EVAL_SCENARIO_PLOT_CFG["basic"])
    sim_duration_s = pcfg["sim_duration_s"]
    slabel         = pcfg["label"]

    methods = ["qos_aware", "availability_based"]
    util_by_mt:  dict = {m: defaultdict(list) for m in methods}
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

    # Paired advantage per timestep
    adv_by_t: dict = defaultdict(list)
    for t in ts:
        q = np.array(sorted(util_by_mt["qos_aware"][t]))
        a = np.array(sorted(util_by_mt["availability_based"].get(t, [0.0])))
        n = min(len(q), len(a))
        if n > 0:
            for delta in (q[:n] - a[:n]):
                adv_by_t[t].append(float(delta))

    # ── 2-panel figure ────────────────────────────────────────────────────────
    fig, (ax_top, ax_bot) = plt.subplots(
        2, 1, figsize=(9, 7),
        sharex=True,
        gridspec_kw={"height_ratios": [3, 2], "hspace": 0.06},
        layout="constrained",
    )

    # Degradation windows (both panels)
    _shade_degrade_windows(ax_top, rows)
    _shade_degrade_windows(ax_bot, rows)

    # — Top panel: utility over time ——————————————————————————————————————————
    styles = {
        "qos_aware":          ("-",  "o", COLOR_PROPOSED, "Proposed — QoS-aware"),
        "availability_based": ("--", "s", COLOR_BASELINE, "Baseline — availability-based"),
    }

    for m in methods:
        if not util_by_mt[m]:
            continue
        ls, mk, color, name = styles[m]
        x, med, p10, p90 = _band(util_by_mt[m], ts)
        _plot_with_band(ax_top, x, med, p10, p90,
                        color=color, linestyle=ls, marker=mk,
                        label_med=f"{name} (median)",
                        label_band=f"{name} (p10 / p90)")

    # Failover rugs at the top of the utility axis
    for m, (ls, mk, color, name) in styles.items():
        fo_ts = [t for t, c in fo_count_mt[m].items() if c > 0]
        if fo_ts:
            ax_top.vlines(fo_ts, 0.95, 1.02, color=color,
                          linewidth=1.0, alpha=0.35, zorder=4)

    ax_top.set_ylabel("Utility", fontsize=11)
    ax_top.set_title(
        f"Controlled Degradation: QoS-Aware vs. Availability-Based  [{slabel}]\n"
        f"(dashed lines: 10th/90th percentile across {n_runs} runs; "
        "gray bands: degradation windows)",
        fontsize=10.5,
    )
    ax_top.set_xlim(0, sim_duration_s)
    ax_top.set_ylim(-0.02, 1.08)

    handles = []
    for m in methods:
        ls, mk, color, name = styles[m]
        handles.append(mlines.Line2D([], [], color=color, linewidth=LW_MAIN,
                                     linestyle=ls, marker=mk, markersize=5,
                                     label=f"{name} (median)"))
        handles.append(mlines.Line2D([], [], color=color, linewidth=LW_BAND,
                                     linestyle="--", alpha=0.70,
                                     label=f"{name} (p10 / p90)"))
    handles.append(mpatches.Patch(color=COLOR_DEGRADE, alpha=0.6,
                                   label="Degradation window (median onset–fail)"))
    ax_top.legend(handles=handles, fontsize=8, loc="lower left",
                  framealpha=0.92, ncol=1)
    _style(ax_top, ylim=(-0.02, 1.10))

    # — Bottom panel: utility advantage ———————————————————————————————————————
    x_adv, med_adv, p10_adv, p90_adv = _band(adv_by_t, ts)

    ax_bot.fill_between(x_adv, p10_adv, p90_adv,
                        color=COLOR_PROPOSED, alpha=BAND_FILL_ALPHA * 1.5, zorder=1)
    ax_bot.plot(x_adv, p10_adv, color=COLOR_PROPOSED, linewidth=LW_BAND,
                linestyle="--", alpha=0.70, zorder=2)
    ax_bot.plot(x_adv, p90_adv, color=COLOR_PROPOSED, linewidth=LW_BAND,
                linestyle="--", alpha=0.70, zorder=2, label="p10 / p90")
    ax_bot.plot(x_adv, med_adv, color=COLOR_PROPOSED, linewidth=LW_MAIN,
                linestyle="-", marker="o",
                markevery=max(1, len(x_adv) // MARKER_EVERY), markersize=4,
                label="Proposed − Baseline (median)", zorder=3)
    ax_bot.axhline(0, color="#374151", linewidth=LW_ZERO,
                   linestyle="--", label="No advantage", zorder=2)

    ax_bot.set_xlabel("Simulation time (s)", fontsize=11)
    ax_bot.set_ylabel("Utility advantage\n(Proposed − Baseline)", fontsize=10)
    ax_bot.legend(fontsize=8, loc="upper left", framealpha=0.92)
    ax_bot.annotate("Shaded region: p10–p90 across runs",
                    xy=(0.62, 0.04), xycoords="axes fraction",
                    fontsize=7.5, color="#6b7280")
    _style(ax_bot)

    _save(fig, output_dir, "degradation_combined")
    print("  [degradation] done")


# ── Plot 3: Failover delay benchmark (existing Go CSV) ───────────────────────

def plot_failover(input_dir: Path, output_dir: Path) -> None:
    # Failover CSV is NOT namespaced by eval-scenario (it comes from Go services).
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

    _plot_with_band(ax, delays, l_avg, l_p10, l_p90,
                    color=COLOR_LOCAL, linestyle="-", marker="o",
                    label_med="Local failover (median)",
                    label_band="Local (p10 / p90)")
    _plot_with_band(ax, delays, c_avg, c_p10, c_p90,
                    color=COLOR_CENTRAL, linestyle="--", marker="s",
                    label_med="Centralised orchestration (median)",
                    label_band="Centralised (p10 / p90)")

    ax.set_xlabel("Simulated one-way network latency (ms)", fontsize=11)
    ax.set_ylabel("Failover decision delay (ms)", fontsize=11)
    ax.set_title("Failover Decision Delay vs. Network Latency\n"
                 "(dashed: 10th/90th percentile)", fontsize=11)
    ax.legend(fontsize=9)
    ax.annotate("Shaded region: p10–p90 across runs",
                xy=(0.02, 0.96), xycoords="axes fraction",
                fontsize=7.5, color="#6b7280", va="top")
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
                   help="Root results directory (eval-scenario subdir appended automatically)")
    p.add_argument("--output-dir", type=Path, default=Path("docs/figures"),
                   help="Root figures directory (eval-scenario subdir appended automatically)")
    p.add_argument("--scenario",
                   choices=["tradeoff", "degradation", "failover", "all"],
                   default="all",
                   help="Which set of plots to generate")
    p.add_argument("--eval-scenario",
                   choices=list(EVAL_SCENARIO_PLOT_CFG.keys()),
                   default="basic",
                   dest="eval_scenario",
                   help="Simulation scenario whose results to plot")
    args = p.parse_args()

    # Namespace input and output by eval-scenario
    effective_input_dir  = args.input_dir  / args.eval_scenario
    effective_output_dir = args.output_dir / args.eval_scenario

    print(f"\n{'='*62}")
    print(f"  Scenario       : {args.scenario}")
    print(f"  Eval scenario  : {args.eval_scenario}")
    print(f"  Input dir      : {effective_input_dir.resolve()}")
    print(f"  Output dir     : {effective_output_dir.resolve()}")
    print(f"{'='*62}\n")

    if args.scenario in ("tradeoff", "all"):
        plot_tradeoff(effective_input_dir, effective_output_dir, args.eval_scenario)
    if args.scenario in ("degradation", "all"):
        plot_degradation(effective_input_dir, effective_output_dir, args.eval_scenario)
    if args.scenario in ("failover", "all"):
        # Failover data is in the root input dir (not scenario-namespaced)
        plot_failover(args.input_dir, effective_output_dir)

    print(f"\nAll figures written to: {effective_output_dir.resolve()}\n")


if __name__ == "__main__":
    main()

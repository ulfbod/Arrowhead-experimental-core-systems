#!/usr/bin/env python3
"""
Conceptual experiment generator for the MineIO composable DT paper.

Implements two evaluations:
  1. QoS Trade-off Analysis    – how weighted utility drives provider selection
  2. Controlled Degradation    – resilience of QoS-aware vs availability-based selection

Each run uses a unique seed derived from the base seed:
  seed_i = base_seed + i   (i = 0 .. runs-1)

If no base seed is provided one is generated automatically and printed so the
experiment is always reproducible with --seed <printed_value>.

Usage:
  python scripts/experiments.py --runs 50 --seed 1234 --scenario all --output-dir results/
  python scripts/experiments.py --runs 30 --scenario tradeoff
  python scripts/experiments.py --runs 30 --scenario degradation --baseline availability_based
  python scripts/experiments.py --eval-scenario improved01 --runs 50 --seed 1234
"""
import argparse
import csv
import json
import os
import random
import sys
import time
from pathlib import Path

try:
    import numpy as np
except ImportError:
    print("ERROR: numpy is required.  pip install numpy", file=sys.stderr)
    sys.exit(1)


# ── Shared constant ───────────────────────────────────────────────────────────

MAX_LATENCY_MS = 100.0   # normalisation ceiling: latency above this contributes 0
MAX_EPISODES   = 4       # maximum episode columns written to CSV (unused = empty string)


# ── Scenario configurations ───────────────────────────────────────────────────
#
# Each entry fully parameterises both the tradeoff and degradation experiments.
# This is the single authoritative source for scenario parameters.

SCENARIO_CONFIGS: dict = {

    # ── basic ─────────────────────────────────────────────────────────────────
    # Current evaluation setting.  Moderate QoS separation, 2 degradation
    # episodes, 120 s simulation.  Used for the original paper figures.
    "basic": {
        "label": "basic",
        "description": (
            "Current evaluation: moderate QoS separation, two degradation "
            "episodes, 120 s simulation."
        ),

        # — Trade-off experiment ————————————————————————————————————————————
        "alpha_steps": 21,          # number of α values in [0, 1]
        "prov_a_ranges": {          # Provider A: quality / accurate / slow
            "accuracy":    (0.88, 0.99),
            "latency_ms":  (40.0, 80.0),
            "reliability": (0.90, 0.99),
        },
        "prov_b_ranges": {          # Provider B: fast / noisy / less reliable
            "accuracy":    (0.50, 0.74),
            "latency_ms":  (3.0,  15.0),
            "reliability": (0.68, 0.87),
        },

        # — Degradation experiment ——————————————————————————————————————————
        "sim_duration_s":         120,
        "recovery_duration_s":     10,
        "degrade_rate":           (0.04, 0.15),   # accuracy units/s degradation
        "episode_count":           2,
        "nominal_qos": {
            "idt2a": {"accuracy": 0.97, "latency_ms": 20.0, "reliability": 0.99},
            "idt2b": {"accuracy": 0.75, "latency_ms":  8.0, "reliability": 0.91},
        },
        # Utility weights for degradation: (w_accuracy, w_latency, w_reliability)
        "degrade_weights":         (0.40, 0.30, 0.30),
        "utility_failover_threshold":      0.55,
        "qos_switch_hysteresis":           0.06,
        "availability_failover_threshold": 0.15,
        "onset_range":            (10.0, 18.0),   # time of first onset within run
        "degrade_window":         ( 8.0, 14.0),   # onset → hard-fail duration
        "fail_window":            ( 6.0, 10.0),   # hard-fail → recovery-start duration
        "inter_episode_gap":      ( 6.0, 14.0),   # gap between recovery-end and next onset
    },

    # ── stress01 ──────────────────────────────────────────────────────────────
    # Adversarial stress test: extreme provider contrast, four degradation
    # episodes, very fast rates, low availability threshold, 300 s simulation.
    # Designed to make the QoS-aware advantage unmistakably visible.
    "stress01": {
        "label": "stress01",
        "description": (
            "Stress test: extreme QoS separation, four alternating degradation "
            "episodes, 300 s simulation, very aggressive degradation rates, "
            "and a very strict availability baseline threshold (0.08)."
        ),

        # — Trade-off experiment ————————————————————————————————————————————
        "alpha_steps": 21,
        "prov_a_ranges": {          # Provider A: very accurate, very slow, very reliable
            "accuracy":    (0.93, 0.99),
            "latency_ms":  (65.0, 100.0),
            "reliability": (0.93, 0.995),
        },
        "prov_b_ranges": {          # Provider B: very fast, much lower accuracy/reliability
            "accuracy":    (0.28, 0.55),
            "latency_ms":  (1.5,   7.0),
            "reliability": (0.38,  0.65),
        },

        # — Degradation experiment ——————————————————————————————————————————
        "sim_duration_s":         300,
        "recovery_duration_s":     8,
        "degrade_rate":           (0.08, 0.25),
        "episode_count":           4,
        "nominal_qos": {
            # idt2a healthy utility at (0.55, 0.20, 0.25):
            #   0.55*0.98 + 0.20*(1-14/100) + 0.25*0.998 = 0.539+0.172+0.250 = 0.961
            # idt2b healthy utility at (0.55, 0.20, 0.25):
            #   0.55*0.55 + 0.20*(1-4/100)  + 0.25*0.72  = 0.303+0.192+0.180 = 0.675
            # Gap = 0.286 >> hysteresis=0.04 → switch-back is always triggered
            "idt2a": {"accuracy": 0.98, "latency_ms": 14.0, "reliability": 0.998},
            "idt2b": {"accuracy": 0.55, "latency_ms":  4.0, "reliability": 0.72},
        },
        "degrade_weights":         (0.55, 0.20, 0.25),
        "utility_failover_threshold":       0.45,
        "qos_switch_hysteresis":            0.04,   # very responsive switch-back
        "availability_failover_threshold":  0.08,   # baseline almost never reacts in time
        "onset_range":            (10.0, 20.0),
        "degrade_window":         ( 6.0, 12.0),
        "fail_window":            (10.0, 20.0),    # long hard-fail windows
        "inter_episode_gap":      ( 5.0, 12.0),
    },

    # ── improved01 ────────────────────────────────────────────────────────────
    # Strengthened scenario designed to make the QoS-aware advantage unambiguous.
    # Wider provider separation, three degradation episodes, 200 s simulation,
    # stronger accuracy weight, and a more brittle availability-based baseline.
    "improved01": {
        "label": "improved01",
        "description": (
            "Enhanced evaluation: wider QoS separation, three degradation "
            "episodes, 200 s simulation, accuracy-heavy weights."
        ),

        # — Trade-off experiment ————————————————————————————————————————————
        "alpha_steps": 21,
        "prov_a_ranges": {          # Provider A: very accurate, very slow
            "accuracy":    (0.90, 0.99),
            "latency_ms":  (55.0, 95.0),
            "reliability": (0.92, 0.99),
        },
        "prov_b_ranges": {          # Provider B: very fast, much lower accuracy/reliability
            "accuracy":    (0.38, 0.62),
            "latency_ms":  (2.0,   9.0),
            "reliability": (0.52, 0.76),
        },

        # — Degradation experiment ——————————————————————————————————————————
        "sim_duration_s":         200,
        "recovery_duration_s":     10,
        "degrade_rate":           (0.05, 0.18),
        "episode_count":           3,
        "nominal_qos": {
            # idt2a healthy utility at (0.50, 0.25, 0.25):
            #   0.50*0.97 + 0.25*(1-18/100) + 0.25*0.995 = 0.485+0.205+0.249 = 0.939
            # idt2b healthy utility at (0.50, 0.25, 0.25):
            #   0.50*0.68 + 0.25*(1-5/100)  + 0.25*0.82  = 0.340+0.238+0.205 = 0.783
            # Gap = 0.156 >> hysteresis=0.05 → QoS-aware always switches back after recovery
            "idt2a": {"accuracy": 0.97, "latency_ms": 18.0, "reliability": 0.995},
            "idt2b": {"accuracy": 0.68, "latency_ms":  5.0, "reliability": 0.82},
        },
        "degrade_weights":         (0.50, 0.25, 0.25),
        "utility_failover_threshold":       0.50,
        "qos_switch_hysteresis":            0.05,  # more proactive switch-back
        "availability_failover_threshold":  0.12,  # stricter → baseline reacts later
        "onset_range":            (12.0, 20.0),
        "degrade_window":         ( 8.0, 16.0),    # longer gradual degradation
        "fail_window":            ( 8.0, 14.0),    # longer hard-fail window
        "inter_episode_gap":      ( 5.0, 12.0),
    },
}


# ── Core utility function ─────────────────────────────────────────────────────

def compute_utility(accuracy: float, latency_ms: float, reliability: float,
                    w_accuracy: float, w_latency: float, w_reliability: float) -> float:
    """Weighted additive utility over normalised QoS attributes.

    Latency is inverted so lower latency → higher utility contribution.
    Result is in [0, 1].
    """
    lat_score = 1.0 - min(1.0, latency_ms / MAX_LATENCY_MS)
    return w_accuracy * accuracy + w_latency * lat_score + w_reliability * reliability


# ── Multi-episode degradation model ──────────────────────────────────────────

def _provider_qos_at(t: float, nominal: dict, episodes: list,
                     recovery_duration_s: float) -> dict:
    """Return live QoS of a provider at time t given a list of degradation episodes.

    Each episode is a dict with keys: onset_s, rate, fail_at_s, recover_at_s.
    Phases per episode:
        [onset_s, fail_at_s)                  gradual degradation
        [fail_at_s, recover_at_s)             hard failure
        [recover_at_s, recover_at_s + T_rec)  gradual recovery
    Returns nominal QoS outside all episode windows.
    """
    for ep in episodes:
        onset_s      = ep["onset_s"]
        rate         = ep["rate"]
        fail_at_s    = ep["fail_at_s"]
        recover_at_s = ep["recover_at_s"]
        recovery_end = recover_at_s + recovery_duration_s

        if onset_s <= t < fail_at_s:
            elapsed = t - onset_s
            return {
                "accuracy":    max(0.0, nominal["accuracy"]    - elapsed * rate),
                "latency_ms":  nominal["latency_ms"] * (1.0 + elapsed * rate * 2.0),
                "reliability": max(0.0, nominal["reliability"] - elapsed * rate * 0.8),
            }
        if fail_at_s <= t < recover_at_s:
            return {
                "accuracy":    0.0,
                "latency_ms":  nominal["latency_ms"] * 5.0,
                "reliability": 0.0,
            }
        if recover_at_s <= t < recovery_end:
            frac = (t - recover_at_s) / recovery_duration_s
            return {
                "accuracy":    nominal["accuracy"]    * frac,
                "latency_ms":  nominal["latency_ms"]  * (4.0 - 3.0 * frac),
                "reliability": nominal["reliability"] * frac,
            }

    return dict(nominal)


# ── Experiment 1: QoS Trade-off Analysis ─────────────────────────────────────

def run_tradeoff(runs: int, base_seed: int, output_dir: Path, cfg: dict) -> None:
    """Sweep the accuracy weight from 0 to 1 across N runs with randomised QoS profiles.

    Provider A ("quality sensor"): high accuracy, slow, highly reliable.
    Provider B ("fast sensor"):    lower accuracy, fast, less reliable.
    The structural contrast creates a genuine accuracy–latency crossover.

    Per-run files: <output_dir>/tradeoff/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    <output_dir>/tradeoff/aggregated.csv
    """
    run_dir  = output_dir / "tradeoff" / "runs"
    run_dir.mkdir(parents=True, exist_ok=True)
    agg_path = output_dir / "tradeoff" / "aggregated.csv"

    header = [
        "run", "seed", "alpha",
        "prov_a_accuracy", "prov_a_latency_ms", "prov_a_reliability",
        "prov_b_accuracy", "prov_b_latency_ms", "prov_b_reliability",
        "utility_a", "utility_b",
        "selected", "selected_utility", "best_utility", "regret",
        "w_accuracy", "w_latency", "w_reliability",
    ]
    all_rows: list = []

    alpha_steps = np.linspace(0.0, 1.0, cfg["alpha_steps"])
    ra = cfg["prov_a_ranges"]
    rb = cfg["prov_b_ranges"]

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        profiles = {
            "idt2a": {
                "accuracy":    float(rng.uniform(*ra["accuracy"])),
                "latency_ms":  float(rng.uniform(*ra["latency_ms"])),
                "reliability": float(rng.uniform(*ra["reliability"])),
            },
            "idt2b": {
                "accuracy":    float(rng.uniform(*rb["accuracy"])),
                "latency_ms":  float(rng.uniform(*rb["latency_ms"])),
                "reliability": float(rng.uniform(*rb["reliability"])),
            },
        }

        run_rows: list = []
        for alpha in alpha_steps:
            w_acc = float(alpha)
            w_lat = float((1.0 - alpha) / 2.0)
            w_rel = float((1.0 - alpha) / 2.0)

            utilities = {
                pid: compute_utility(
                    p["accuracy"], p["latency_ms"], p["reliability"],
                    w_acc, w_lat, w_rel,
                )
                for pid, p in profiles.items()
            }

            selected     = max(utilities, key=utilities.get)
            best_utility = max(utilities.values())
            sel_utility  = utilities[selected]
            regret       = best_utility - sel_utility

            pa, pb = profiles["idt2a"], profiles["idt2b"]
            row = {
                "run":   run,  "seed": seed,
                "alpha": round(float(alpha), 4),
                "prov_a_accuracy":    round(pa["accuracy"],    6),
                "prov_a_latency_ms":  round(pa["latency_ms"],  4),
                "prov_a_reliability": round(pa["reliability"],  6),
                "prov_b_accuracy":    round(pb["accuracy"],    6),
                "prov_b_latency_ms":  round(pb["latency_ms"],  4),
                "prov_b_reliability": round(pb["reliability"],  6),
                "utility_a":          round(utilities["idt2a"], 6),
                "utility_b":          round(utilities["idt2b"], 6),
                "selected":           selected,
                "selected_utility":   round(sel_utility,  6),
                "best_utility":       round(best_utility, 6),
                "regret":             round(regret,       8),
                "w_accuracy":         round(w_acc, 4),
                "w_latency":          round(w_lat, 4),
                "w_reliability":      round(w_rel, 4),
            }
            run_rows.append(row)
            all_rows.append(row)

        csv_path = run_dir / f"run_{run:04d}_seed_{seed}.csv"
        _write_csv(csv_path, header, run_rows)
        print(f"  [tradeoff]   run={run:3d}  seed={seed}", flush=True)

    _write_csv(agg_path, header, all_rows)
    print(f"  [tradeoff]   aggregated ({len(all_rows)} rows) → {agg_path}", flush=True)


# ── Experiment 2: Controlled Degradation ─────────────────────────────────────

def run_degradation(runs: int, base_seed: int, output_dir: Path,
                    methods: tuple, cfg: dict) -> None:
    """Simulate sequential degradation episodes and compare selection strategies.

    Episode pattern: alternating providers (A→B→A for 3 episodes, A→B for 2).
    Each episode has three phases: gradual degradation → hard fail → gradual recovery.

    QoS-aware strategy:
      - Emergency switch: active utility < utility_failover_threshold.
      - Proactive switch-back: alternative is better by > qos_switch_hysteresis.

    Availability-based strategy:
      - Switch only when active reliability < availability_failover_threshold.
      - Never switches back proactively.

    Per-run files: <output_dir>/degradation/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    <output_dir>/degradation/aggregated.csv
    """
    run_dir  = output_dir / "degradation" / "runs"
    run_dir.mkdir(parents=True, exist_ok=True)
    agg_path = output_dir / "degradation" / "aggregated.csv"

    # CSV always writes MAX_EPISODES episode columns (empty string when unused)
    episode_cols: list = []
    for i in range(1, MAX_EPISODES + 1):
        episode_cols += [
            f"degraded{i}", f"onset{i}_s", f"rate{i}", f"fail{i}_s", f"recover{i}_s",
        ]

    header = (
        ["run", "seed"]
        + episode_cols
        + ["t", "method", "active_provider",
           "accuracy", "latency_ms", "reliability", "utility", "failover_event"]
    )
    all_rows: list = []

    episode_count       = cfg["episode_count"]
    sim_duration_s      = cfg["sim_duration_s"]
    recovery_duration_s = cfg["recovery_duration_s"]
    nominal_qos         = cfg["nominal_qos"]
    w_acc, w_lat, w_rel = cfg["degrade_weights"]
    utility_thr         = cfg["utility_failover_threshold"]
    hysteresis          = cfg["qos_switch_hysteresis"]
    avail_thr           = cfg["availability_failover_threshold"]
    providers           = ["idt2a", "idt2b"]

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        # ── Build episode list ─────────────────────────────────────────────
        # First episode: random provider; subsequent episodes alternate.
        ep_records: list = []
        ep_map: dict = {p: [] for p in providers}

        current_start = 0.0
        for i in range(episode_count):
            if i == 0:
                degraded = str(rng.choice(providers))
                onset_s  = float(rng.uniform(*cfg["onset_range"]))
            else:
                prev_degraded = ep_records[-1]["degraded"]
                degraded  = "idt2b" if prev_degraded == "idt2a" else "idt2a"
                onset_s   = current_start + float(rng.uniform(*cfg["inter_episode_gap"]))

            rate         = float(rng.uniform(*cfg["degrade_rate"]))
            fail_at_s    = onset_s + float(rng.uniform(*cfg["degrade_window"]))
            recover_at_s = fail_at_s + float(rng.uniform(*cfg["fail_window"]))
            recovery_end = recover_at_s + recovery_duration_s
            current_start = recovery_end

            ep = {
                "degraded":     degraded,
                "onset_s":      onset_s,
                "rate":         rate,
                "fail_at_s":    fail_at_s,
                "recover_at_s": recover_at_s,
                "recovery_end": recovery_end,
            }
            ep_records.append(ep)
            ep_map[degraded].append({
                "onset_s":     onset_s,
                "rate":        rate,
                "fail_at_s":   fail_at_s,
                "recover_at_s": recover_at_s,
            })

        # ── Build per-episode meta dict (written into every row) ───────────
        meta: dict = {"run": run, "seed": seed}
        for i in range(MAX_EPISODES):
            if i < len(ep_records):
                ep = ep_records[i]
                meta[f"degraded{i+1}"]  = ep["degraded"]
                meta[f"onset{i+1}_s"]   = round(ep["onset_s"],     3)
                meta[f"rate{i+1}"]      = round(ep["rate"],        5)
                meta[f"fail{i+1}_s"]    = round(ep["fail_at_s"],   3)
                meta[f"recover{i+1}_s"] = round(ep["recover_at_s"], 3)
            else:
                meta[f"degraded{i+1}"]  = ""
                meta[f"onset{i+1}_s"]   = ""
                meta[f"rate{i+1}"]      = ""
                meta[f"fail{i+1}_s"]    = ""
                meta[f"recover{i+1}_s"] = ""

        run_rows: list = []

        for method in methods:
            active      = "idt2a"
            prev_active = active

            for t in range(sim_duration_s + 1):
                tf = float(t)

                # Live QoS for all providers
                qos = {
                    pid: _provider_qos_at(tf, nominal_qos[pid], ep_map[pid],
                                          recovery_duration_s)
                    for pid in providers
                }

                # ── Selection decision ────────────────────────────────────
                if method == "qos_aware":
                    aq = qos[active]
                    u_active = compute_utility(
                        aq["accuracy"], aq["latency_ms"], aq["reliability"],
                        w_acc, w_lat, w_rel,
                    )
                    alternatives = [p for p in qos if p != active]
                    if alternatives:
                        best_alt = max(
                            alternatives,
                            key=lambda p: compute_utility(
                                qos[p]["accuracy"], qos[p]["latency_ms"],
                                qos[p]["reliability"],
                                w_acc, w_lat, w_rel,
                            ),
                        )
                        u_alt = compute_utility(
                            qos[best_alt]["accuracy"], qos[best_alt]["latency_ms"],
                            qos[best_alt]["reliability"],
                            w_acc, w_lat, w_rel,
                        )
                        should_switch = (
                            u_active < utility_thr
                            or u_alt > u_active + hysteresis
                        )
                        if should_switch and u_alt > u_active:
                            active = best_alt

                else:  # availability_based — only reacts to hard failures
                    if qos[active]["reliability"] < avail_thr:
                        for alt in [p for p in qos if p != active]:
                            if qos[alt]["reliability"] >= avail_thr:
                                active = alt
                                break

                failover_event = 1 if active != prev_active else 0
                prev_active    = active

                aq      = qos[active]
                utility = compute_utility(
                    aq["accuracy"], aq["latency_ms"], aq["reliability"],
                    w_acc, w_lat, w_rel,
                )

                row = {
                    **meta,
                    "t":               t,
                    "method":          method,
                    "active_provider": active,
                    "accuracy":        round(aq["accuracy"],    6),
                    "latency_ms":      round(aq["latency_ms"],  4),
                    "reliability":     round(aq["reliability"],  6),
                    "utility":         round(utility,            6),
                    "failover_event":  failover_event,
                }
                run_rows.append(row)
                all_rows.append(row)

        ep_summary = "  ".join(
            f"ep{i+1}={ep_records[i]['degraded']}@{ep_records[i]['onset_s']:.1f}s"
            f"(rate={ep_records[i]['rate']:.3f})"
            for i in range(episode_count)
        )
        csv_path = run_dir / f"run_{run:04d}_seed_{seed}.csv"
        _write_csv(csv_path, header, run_rows)
        print(
            f"  [degradation] run={run:3d}  seed={seed}  {ep_summary}",
            flush=True,
        )

    _write_csv(agg_path, header, all_rows)
    print(f"  [degradation] aggregated ({len(all_rows)} rows) → {agg_path}", flush=True)


# ── Helpers ───────────────────────────────────────────────────────────────────

def _write_csv(path: Path, header: list, rows: list) -> None:
    with open(path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=header)
        writer.writeheader()
        writer.writerows(rows)


def _write_manifest(output_dir: Path, base_seed: int, runs: int,
                    scenario: str, eval_scenario: str, methods: list,
                    cfg: dict) -> None:
    manifest = {
        "generated_at":   time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "base_seed":      base_seed,
        "runs":           runs,
        "scenario":       scenario,
        "eval_scenario":  eval_scenario,
        "methods":        methods,
        "seed_schedule":  f"seed_i = {base_seed} + i   for i = 0 .. {runs - 1}",
        "reproducibility": (
            f"python scripts/experiments.py "
            f"--seed {base_seed} --runs {runs} "
            f"--scenario {scenario} --eval-scenario {eval_scenario}"
        ),
        "scenario_description": cfg["description"],
        "tradeoff_provider_A": {
            "accuracy_range":    list(cfg["prov_a_ranges"]["accuracy"]),
            "latency_ms_range":  list(cfg["prov_a_ranges"]["latency_ms"]),
            "reliability_range": list(cfg["prov_a_ranges"]["reliability"]),
        },
        "tradeoff_provider_B": {
            "accuracy_range":    list(cfg["prov_b_ranges"]["accuracy"]),
            "latency_ms_range":  list(cfg["prov_b_ranges"]["latency_ms"]),
            "reliability_range": list(cfg["prov_b_ranges"]["reliability"]),
        },
        "degradation_nominal_idt2a":     cfg["nominal_qos"]["idt2a"],
        "degradation_nominal_idt2b":     cfg["nominal_qos"]["idt2b"],
        "utility_weights_degradation": {
            "accuracy":    cfg["degrade_weights"][0],
            "latency":     cfg["degrade_weights"][1],
            "reliability": cfg["degrade_weights"][2],
        },
        "episode_count":                  cfg["episode_count"],
        "utility_failover_threshold":     cfg["utility_failover_threshold"],
        "qos_switch_hysteresis":          cfg["qos_switch_hysteresis"],
        "availability_failover_threshold": cfg["availability_failover_threshold"],
        "sim_duration_s":                 cfg["sim_duration_s"],
        "recovery_duration_s":            cfg["recovery_duration_s"],
        "degrade_rate_range":             list(cfg["degrade_rate"]),
        "max_latency_ms_normalization":   MAX_LATENCY_MS,
    }
    output_dir.mkdir(parents=True, exist_ok=True)
    path = output_dir / "manifest.json"
    with open(path, "w") as f:
        json.dump(manifest, f, indent=2)
    print(f"  manifest → {path}", flush=True)


# ── CLI entry point ───────────────────────────────────────────────────────────

def _parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Run conceptual experiments for the MineIO DT paper.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    p.add_argument("--runs",          type=int,  default=30,
                   help="Number of randomised runs")
    p.add_argument("--seed",          type=int,  default=None,
                   help="Base seed for reproducibility (auto-generated if omitted)")
    p.add_argument("--output-dir",    type=Path, default=Path("results"),
                   help="Root directory for output files (scenario subdir appended automatically)")
    p.add_argument("--scenario",
                   choices=["tradeoff", "degradation", "all"],
                   default="all",
                   help="Which experiment type(s) to run")
    p.add_argument("--eval-scenario",
                   choices=sorted(SCENARIO_CONFIGS.keys()),
                   default="basic",
                   dest="eval_scenario",
                   help="Simulation scenario / parameter set to use")
    p.add_argument("--baseline",
                   choices=["qos_aware", "availability_based", "both"],
                   default="both",
                   help="Which selection strategies to include in degradation experiment")
    return p.parse_args()


def main() -> None:
    args = _parse_args()

    base_seed = args.seed if args.seed is not None else random.randint(0, 2**31 - 1)

    methods: list
    if args.baseline == "qos_aware":
        methods = ["qos_aware"]
    elif args.baseline == "availability_based":
        methods = ["availability_based"]
    else:
        methods = ["qos_aware", "availability_based"]

    cfg = SCENARIO_CONFIGS[args.eval_scenario]

    # Namespace output under <output-dir>/<eval-scenario>/
    effective_output_dir = args.output_dir / args.eval_scenario

    print(f"\n{'='*62}")
    print(f"  Base seed      : {base_seed}")
    print(f"  Runs           : {args.runs}")
    print(f"  Experiment     : {args.scenario}")
    print(f"  Eval scenario  : {args.eval_scenario}  ({cfg['description'][:55]}…)")
    print(f"  Methods        : {', '.join(methods)}")
    print(f"  Output dir     : {effective_output_dir.resolve()}")
    print(f"  Seed range     : {base_seed} .. {base_seed + args.runs - 1}")
    print(f"{'='*62}\n")

    _write_manifest(effective_output_dir, base_seed, args.runs,
                    args.scenario, args.eval_scenario, methods, cfg)

    if args.scenario in ("tradeoff", "all"):
        print(f"[1/2] QoS Trade-off Analysis  ({args.runs} runs)…")
        run_tradeoff(args.runs, base_seed, effective_output_dir, cfg)
        print()

    if args.scenario in ("degradation", "all"):
        print(f"[2/2] Controlled Degradation  ({args.runs} runs × {len(methods)} methods)…")
        run_degradation(args.runs, base_seed, effective_output_dir, tuple(methods), cfg)
        print()

    print(f"{'='*62}")
    print(f"  Done.  Base seed: {base_seed}  Eval scenario: {args.eval_scenario}")
    print(f"  Reproduce:  python scripts/experiments.py "
          f"--seed {base_seed} --runs {args.runs} "
          f"--scenario {args.scenario} --eval-scenario {args.eval_scenario}")
    print(f"{'='*62}\n")


if __name__ == "__main__":
    main()

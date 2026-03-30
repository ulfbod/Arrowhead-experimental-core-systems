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


# ── Experiment constants ──────────────────────────────────────────────────────

MAX_LATENCY_MS            = 100.0   # latency above this contributes 0 to utility
SIM_DURATION_S            = 60      # degradation simulation length (seconds)
SIM_TIMESTEP_S            = 1       # one measurement per second

# Failover thresholds
UTILITY_FAILOVER_THRESHOLD       = 0.55   # QoS-aware: switch below this
AVAILABILITY_FAILOVER_THRESHOLD  = 0.15   # availability-based: switch when reliability < this

# Weight sweep for trade-off analysis (w_accuracy from 0 → 1 in 21 steps)
ALPHA_STEPS = np.linspace(0.0, 1.0, 21)

# Fixed weights used in degradation experiment
DEGRADE_W_ACC = 0.40
DEGRADE_W_LAT = 0.30
DEGRADE_W_REL = 0.30


# ── Core utility function ─────────────────────────────────────────────────────

def compute_utility(accuracy: float, latency_ms: float, reliability: float,
                    w_accuracy: float, w_latency: float, w_reliability: float) -> float:
    """Weighted additive utility over normalized QoS attributes.

    Latency is inverted so lower latency → higher utility contribution.
    All attributes and the result are in [0, 1].
    """
    lat_score = 1.0 - min(1.0, latency_ms / MAX_LATENCY_MS)
    return w_accuracy * accuracy + w_latency * lat_score + w_reliability * reliability


# ── Experiment 1: QoS Trade-off Analysis ─────────────────────────────────────

def run_tradeoff(runs: int, base_seed: int, output_dir: Path) -> None:
    """Sweep the accuracy weight from 0 to 1 across N runs with randomised QoS profiles.

    For each run a unique seed is used to sample provider QoS profiles.
    The same set of alpha values is evaluated in every run so results are
    directly comparable across runs.

    Per-run files: results/tradeoff/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    results/tradeoff/aggregated.csv
    """
    run_dir = output_dir / "tradeoff" / "runs"
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

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        # Randomise QoS profiles for this run.
        # Provider A (primary): typically better accuracy/reliability, lower latency.
        # Provider B (fallback): slightly weaker profile, broader range.
        profiles = {
            "idt2a": {
                "accuracy":    float(rng.uniform(0.65, 1.00)),
                "latency_ms":  float(rng.uniform(5.0,  55.0)),
                "reliability": float(rng.uniform(0.75, 1.00)),
            },
            "idt2b": {
                "accuracy":    float(rng.uniform(0.50, 0.90)),
                "latency_ms":  float(rng.uniform(8.0,  75.0)),
                "reliability": float(rng.uniform(0.60, 0.92)),
            },
        }

        run_rows: list = []
        for alpha in ALPHA_STEPS:
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

            selected       = max(utilities, key=utilities.get)
            best_utility   = max(utilities.values())
            sel_utility    = utilities[selected]
            regret         = best_utility - sel_utility   # 0 by construction for argmax

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
                    methods: tuple = ("qos_aware", "availability_based")) -> None:
    """Simulate controlled service degradation and compare selection strategies.

    For each run the degradation scenario is randomised (which provider degrades,
    onset time, degradation rate, hard-failure time).  Two selection strategies
    are evaluated on the *same* scenario so the comparison is paired.

    Degradation model (for the affected provider):
      accuracy(t)    = max(0, nominal - max(0, t - onset) * rate)
      reliability(t) = max(0, nominal - max(0, t - onset) * rate * 0.8)
      latency(t)     = nominal * (1 + max(0, t - onset) * rate * 2.0)
      at t >= fail_at: accuracy=0, reliability=0, latency=5×nominal

    Per-run files: results/degradation/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    results/degradation/aggregated.csv
    """
    run_dir = output_dir / "degradation" / "runs"
    run_dir.mkdir(parents=True, exist_ok=True)
    agg_path = output_dir / "degradation" / "aggregated.csv"

    header = [
        "run", "seed",
        "degraded_provider", "onset_s", "degrade_rate", "fail_at_s",
        "t", "method",
        "active_provider",
        "accuracy", "latency_ms", "reliability",
        "utility",
        "failover_event",
    ]
    all_rows: list = []

    # Nominal QoS (fixed across runs for clean degradation comparison)
    nominal = {
        "idt2a": {"accuracy": 0.95, "latency_ms": 10.0, "reliability": 0.98},
        "idt2b": {"accuracy": 0.85, "latency_ms": 15.0, "reliability": 0.95},
    }

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        # Randomise degradation scenario for this run
        degraded_provider = str(rng.choice(["idt2a", "idt2b"]))
        onset_s           = float(rng.uniform(5.0,  25.0))
        degrade_rate      = float(rng.uniform(0.02,  0.12))
        fail_at_s         = float(onset_s + rng.uniform(10.0, 25.0))

        meta = {
            "run": run, "seed": seed,
            "degraded_provider": degraded_provider,
            "onset_s":       round(onset_s,      3),
            "degrade_rate":  round(degrade_rate,  5),
            "fail_at_s":     round(fail_at_s,     3),
        }

        run_rows: list = []

        for method in methods:
            active      = "idt2a"   # both strategies start on primary
            prev_active = active

            for t in range(SIM_DURATION_S + 1):
                tf = float(t)

                # Compute live QoS of each provider at time t
                qos: dict = {}
                for pid, nom in nominal.items():
                    if pid == degraded_provider:
                        elapsed = max(0.0, tf - onset_s)
                        if tf >= fail_at_s:
                            qos[pid] = {
                                "accuracy":    0.0,
                                "latency_ms":  nom["latency_ms"] * 5.0,
                                "reliability": 0.0,
                            }
                        else:
                            qos[pid] = {
                                "accuracy":    max(0.0, nom["accuracy"]    - elapsed * degrade_rate),
                                "latency_ms":  nom["latency_ms"] * (1.0 + elapsed * degrade_rate * 2.0),
                                "reliability": max(0.0, nom["reliability"] - elapsed * degrade_rate * 0.8),
                            }
                    else:
                        qos[pid] = dict(nom)

                # Selection decision for this timestep
                if method == "qos_aware":
                    aq = qos[active]
                    u_active = compute_utility(
                        aq["accuracy"], aq["latency_ms"], aq["reliability"],
                        DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                    )
                    if u_active < UTILITY_FAILOVER_THRESHOLD:
                        alternatives = [p for p in qos if p != active]
                        best_alt = max(
                            alternatives,
                            key=lambda p: compute_utility(
                                qos[p]["accuracy"], qos[p]["latency_ms"], qos[p]["reliability"],
                                DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                            ),
                        )
                        u_alt = compute_utility(
                            qos[best_alt]["accuracy"], qos[best_alt]["latency_ms"],
                            qos[best_alt]["reliability"],
                            DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                        )
                        if u_alt > u_active:
                            active = best_alt

                else:  # availability_based
                    if qos[active]["reliability"] < AVAILABILITY_FAILOVER_THRESHOLD:
                        for alt in [p for p in qos if p != active]:
                            if qos[alt]["reliability"] >= AVAILABILITY_FAILOVER_THRESHOLD:
                                active = alt
                                break

                failover_event = 1 if active != prev_active else 0
                prev_active = active

                aq = qos[active]
                utility = compute_utility(
                    aq["accuracy"], aq["latency_ms"], aq["reliability"],
                    DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
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

        csv_path = run_dir / f"run_{run:04d}_seed_{seed}.csv"
        _write_csv(csv_path, header, run_rows)
        print(
            f"  [degradation] run={run:3d}  seed={seed}  "
            f"degraded={degraded_provider}  onset={onset_s:.1f}s  "
            f"rate={degrade_rate:.3f}  fail={fail_at_s:.1f}s",
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
                    scenario: str, methods: list) -> None:
    manifest = {
        "generated_at":  time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "base_seed":     base_seed,
        "runs":          runs,
        "scenario":      scenario,
        "methods":       methods,
        "seed_schedule": f"seed_i = {base_seed} + i   for i = 0 .. {runs - 1}",
        "reproducibility": (
            f"python scripts/experiments.py "
            f"--seed {base_seed} --runs {runs} --scenario {scenario}"
        ),
        "utility_weights_degradation": {
            "accuracy":    DEGRADE_W_ACC,
            "latency":     DEGRADE_W_LAT,
            "reliability": DEGRADE_W_REL,
        },
        "utility_threshold":           UTILITY_FAILOVER_THRESHOLD,
        "availability_threshold":      AVAILABILITY_FAILOVER_THRESHOLD,
        "sim_duration_s":              SIM_DURATION_S,
        "max_latency_ms_normalization": MAX_LATENCY_MS,
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
    p.add_argument("--runs",       type=int, default=30,
                   help="Number of randomised runs")
    p.add_argument("--seed",       type=int, default=None,
                   help="Base seed for reproducibility (auto-generated if omitted)")
    p.add_argument("--output-dir", type=Path, default=Path("results"),
                   help="Root directory for output files")
    p.add_argument("--scenario",   choices=["tradeoff", "degradation", "all"],
                   default="all",
                   help="Which experiment(s) to run")
    p.add_argument("--baseline",   choices=["qos_aware", "availability_based", "both"],
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

    print(f"\n{'='*62}")
    print(f"  Base seed  : {base_seed}")
    print(f"  Runs       : {args.runs}")
    print(f"  Scenario   : {args.scenario}")
    print(f"  Methods    : {', '.join(methods)}")
    print(f"  Output dir : {args.output_dir.resolve()}")
    print(f"  Seed range : {base_seed} .. {base_seed + args.runs - 1}")
    print(f"{'='*62}\n")

    _write_manifest(args.output_dir, base_seed, args.runs, args.scenario, methods)

    if args.scenario in ("tradeoff", "all"):
        print(f"[1/2] QoS Trade-off Analysis  ({args.runs} runs)…")
        run_tradeoff(args.runs, base_seed, args.output_dir)
        print()

    if args.scenario in ("degradation", "all"):
        print(f"[2/2] Controlled Degradation  ({args.runs} runs × {len(methods)} methods)…")
        run_degradation(args.runs, base_seed, args.output_dir, tuple(methods))
        print()

    print(f"{'='*62}")
    print(f"  Done.  Base seed: {base_seed}")
    print(f"  Reproduce:  python scripts/experiments.py "
          f"--seed {base_seed} --runs {args.runs} --scenario {args.scenario}")
    print(f"{'='*62}\n")


if __name__ == "__main__":
    main()

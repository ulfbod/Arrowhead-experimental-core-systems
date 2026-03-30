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

MAX_LATENCY_MS = 100.0   # latency above this contributes 0 to utility

# Degradation simulation
SIM_DURATION_S          = 120    # 2 minutes: enough for two degradation episodes
RECOVERY_DURATION_S     = 10     # seconds of gradual recovery after hard fail

# Degradation rates (accuracy units per second)
DEGRADE_RATE_MIN = 0.04
DEGRADE_RATE_MAX = 0.15

# Failover thresholds
UTILITY_FAILOVER_THRESHOLD      = 0.55   # QoS-aware emergency switch
QOS_SWITCH_HYSTERESIS           = 0.06   # QoS-aware proactive switch-back margin
AVAILABILITY_FAILOVER_THRESHOLD = 0.15   # availability-based: switch when reliability < this

# Weight sweep for trade-off analysis (w_accuracy from 0 → 1 in 21 steps)
ALPHA_STEPS = np.linspace(0.0, 1.0, 21)

# Fixed weights used in degradation experiment
DEGRADE_W_ACC = 0.40
DEGRADE_W_LAT = 0.30
DEGRADE_W_REL = 0.30

# ── Nominal QoS for degradation experiment ────────────────────────────────────
# idt2a: high-accuracy/slow quality sensor — clearly preferred when healthy
# idt2b: low-latency/fast sensor — useful fallback, lower quality
#
# Healthy utilities (w_acc=0.40, w_lat=0.30, w_rel=0.30):
#   idt2a = 0.40*0.97 + 0.30*(1-20/100) + 0.30*0.99 = 0.388+0.240+0.297 = 0.925
#   idt2b = 0.40*0.75 + 0.30*(1- 8/100) + 0.30*0.91 = 0.300+0.276+0.273 = 0.849
#   Gap = 0.076 > QOS_SWITCH_HYSTERESIS (0.06) → QoS-aware switches back after recovery
NOMINAL_QOS = {
    "idt2a": {"accuracy": 0.97, "latency_ms": 20.0, "reliability": 0.99},
    "idt2b": {"accuracy": 0.75, "latency_ms":  8.0, "reliability": 0.91},
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

def _provider_qos_at(t: float, nominal: dict, episodes: list) -> dict:
    """Return live QoS of a provider at time t given a list of degradation episodes.

    Each episode is a dict with keys:
        onset_s, rate, fail_at_s, recover_at_s
    Phases per episode:
        [onset_s, fail_at_s)         gradual degradation
        [fail_at_s, recover_at_s)    hard failure (zero accuracy/reliability)
        [recover_at_s, recover_at_s + RECOVERY_DURATION_S)  gradual recovery
    Nominal QoS outside all episode windows.
    """
    for ep in episodes:
        onset_s, rate = ep["onset_s"], ep["rate"]
        fail_at_s     = ep["fail_at_s"]
        recover_at_s  = ep["recover_at_s"]
        recovery_end  = recover_at_s + RECOVERY_DURATION_S

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
            frac = (t - recover_at_s) / RECOVERY_DURATION_S
            return {
                "accuracy":    nominal["accuracy"]    * frac,
                "latency_ms":  nominal["latency_ms"]  * (4.0 - 3.0 * frac),  # 4× → 1×
                "reliability": nominal["reliability"] * frac,
            }

    return dict(nominal)


# ── Experiment 1: QoS Trade-off Analysis ─────────────────────────────────────

def run_tradeoff(runs: int, base_seed: int, output_dir: Path) -> None:
    """Sweep the accuracy weight from 0 to 1 across N runs with randomised QoS profiles.

    Provider A ("quality sensor"): high accuracy, slow, highly reliable.
    Provider B ("fast sensor"):    lower accuracy, fast, less reliable.
    The structural contrast creates a genuine accuracy–latency crossover near α ≈ 0.35.

    Per-run files: results/tradeoff/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    results/tradeoff/aggregated.csv
    """
    run_dir = output_dir / "tradeoff" / "runs"
    run_dir.mkdir(parents=True, exist_ok=True)
    agg_path = output_dir / "tradeoff" / "aggregated.csv"

    header = [
        "run", "seed", "alpha",
        # Provider A: accurate/slow (quality sensor)
        "prov_a_accuracy", "prov_a_latency_ms", "prov_a_reliability",
        # Provider B: fast/noisy (speed sensor)
        "prov_b_accuracy", "prov_b_latency_ms", "prov_b_reliability",
        "utility_a", "utility_b",
        "selected", "selected_utility", "best_utility", "regret",
        "w_accuracy", "w_latency", "w_reliability",
    ]
    all_rows: list = []

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        # Provider A: high accuracy, high reliability, slow (quality sensor).
        # Provider B: low accuracy, low reliability, fast (speed sensor).
        # The ranges are intentionally non-overlapping in the latency dimension
        # so the accuracy–latency trade-off is always visible.
        profiles = {
            "idt2a": {
                "accuracy":    float(rng.uniform(0.88, 0.99)),   # always high
                "latency_ms":  float(rng.uniform(40.0, 80.0)),   # always slow
                "reliability": float(rng.uniform(0.90, 0.99)),   # always reliable
            },
            "idt2b": {
                "accuracy":    float(rng.uniform(0.50, 0.74)),   # always lower
                "latency_ms":  float(rng.uniform(3.0,  15.0)),   # always fast
                "reliability": float(rng.uniform(0.68, 0.87)),   # always less reliable
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
                    methods: tuple = ("qos_aware", "availability_based")) -> None:
    """Simulate two sequential degradation episodes and compare selection strategies.

    Episode 1: one provider degrades, hard-fails, then recovers.
    Episode 2: the OTHER provider degrades, hard-fails, then recovers.
    Both episodes use the same RNG-drawn parameters but affect different providers,
    so the QoS-aware method faces two distinct adaptation challenges per run.

    QoS-aware strategy:
      - Switch if active utility < UTILITY_FAILOVER_THRESHOLD (emergency).
      - Switch if an alternative is better by > QOS_SWITCH_HYSTERESIS (proactive).
      This enables switch-back to the primary after it recovers.

    Availability-based strategy:
      - Switch only when active reliability < AVAILABILITY_FAILOVER_THRESHOLD.
      - Never switches back proactively — only reacts to hard failures.

    Per-run files: results/degradation/runs/run_NNNN_seed_SSSS.csv
    Aggregated:    results/degradation/aggregated.csv
    """
    run_dir = output_dir / "degradation" / "runs"
    run_dir.mkdir(parents=True, exist_ok=True)
    agg_path = output_dir / "degradation" / "aggregated.csv"

    header = [
        "run", "seed",
        # Episode 1
        "degraded1", "onset1_s", "rate1", "fail1_s", "recover1_s",
        # Episode 2
        "degraded2", "onset2_s", "rate2", "fail2_s", "recover2_s",
        # Timestep
        "t", "method",
        "active_provider",
        "accuracy", "latency_ms", "reliability",
        "utility",
        "failover_event",
    ]
    all_rows: list = []

    for run in range(runs):
        seed = base_seed + run
        rng  = np.random.default_rng(seed)

        # ── Episode 1 ──────────────────────────────────────────────────────────
        providers    = ["idt2a", "idt2b"]
        degraded1    = str(rng.choice(providers))
        degraded2    = "idt2b" if degraded1 == "idt2a" else "idt2a"

        onset1_s     = float(rng.uniform(10.0, 18.0))
        rate1        = float(rng.uniform(DEGRADE_RATE_MIN, DEGRADE_RATE_MAX))
        fail1_s      = float(onset1_s  + rng.uniform(8.0,  14.0))
        recover1_s   = float(fail1_s   + rng.uniform(6.0,  10.0))
        recovery_end1 = recover1_s + RECOVERY_DURATION_S

        # ── Episode 2 starts after provider 1 has fully recovered ─────────────
        onset2_s     = float(recovery_end1 + rng.uniform(6.0, 14.0))
        rate2        = float(rng.uniform(DEGRADE_RATE_MIN, DEGRADE_RATE_MAX))
        fail2_s      = float(onset2_s  + rng.uniform(8.0,  14.0))
        recover2_s   = float(fail2_s   + rng.uniform(6.0,  10.0))

        # Episode tables per provider
        ep_map: dict = {"idt2a": [], "idt2b": []}
        ep_map[degraded1].append({
            "onset_s": onset1_s, "rate": rate1,
            "fail_at_s": fail1_s, "recover_at_s": recover1_s,
        })
        ep_map[degraded2].append({
            "onset_s": onset2_s, "rate": rate2,
            "fail_at_s": fail2_s, "recover_at_s": recover2_s,
        })

        meta = {
            "run": run, "seed": seed,
            "degraded1":  degraded1,
            "onset1_s":   round(onset1_s,   3),
            "rate1":      round(rate1,       5),
            "fail1_s":    round(fail1_s,     3),
            "recover1_s": round(recover1_s,  3),
            "degraded2":  degraded2,
            "onset2_s":   round(onset2_s,    3),
            "rate2":      round(rate2,        5),
            "fail2_s":    round(fail2_s,      3),
            "recover2_s": round(recover2_s,   3),
        }

        run_rows: list = []

        for method in methods:
            active      = "idt2a"
            prev_active = active

            for t in range(SIM_DURATION_S + 1):
                tf = float(t)

                # Live QoS at this timestep
                qos = {
                    pid: _provider_qos_at(tf, NOMINAL_QOS[pid], ep_map[pid])
                    for pid in providers
                }

                # ── Selection decision ────────────────────────────────────────
                if method == "qos_aware":
                    aq      = qos[active]
                    u_active = compute_utility(
                        aq["accuracy"], aq["latency_ms"], aq["reliability"],
                        DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                    )
                    alternatives = [p for p in qos if p != active]
                    if alternatives:
                        best_alt = max(
                            alternatives,
                            key=lambda p: compute_utility(
                                qos[p]["accuracy"], qos[p]["latency_ms"],
                                qos[p]["reliability"],
                                DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                            ),
                        )
                        u_alt = compute_utility(
                            qos[best_alt]["accuracy"], qos[best_alt]["latency_ms"],
                            qos[best_alt]["reliability"],
                            DEGRADE_W_ACC, DEGRADE_W_LAT, DEGRADE_W_REL,
                        )
                        # Emergency switch (below threshold) OR proactive switch-back
                        should_switch = (
                            u_active < UTILITY_FAILOVER_THRESHOLD
                            or u_alt > u_active + QOS_SWITCH_HYSTERESIS
                        )
                        if should_switch and u_alt > u_active:
                            active = best_alt

                else:  # availability_based — only reacts to hard failures
                    if qos[active]["reliability"] < AVAILABILITY_FAILOVER_THRESHOLD:
                        for alt in [p for p in qos if p != active]:
                            if qos[alt]["reliability"] >= AVAILABILITY_FAILOVER_THRESHOLD:
                                active = alt
                                break

                failover_event = 1 if active != prev_active else 0
                prev_active    = active

                aq      = qos[active]
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
            f"ep1={degraded1}@{onset1_s:.1f}s(rate={rate1:.3f})  "
            f"ep2={degraded2}@{onset2_s:.1f}s(rate={rate2:.3f})",
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
        "tradeoff_provider_A": "quality sensor: high acc [0.88-0.99], slow [40-80ms], reliable [0.90-0.99]",
        "tradeoff_provider_B": "fast sensor: low acc [0.50-0.74], fast [3-15ms], less reliable [0.68-0.87]",
        "degradation_nominal_idt2a": NOMINAL_QOS["idt2a"],
        "degradation_nominal_idt2b": NOMINAL_QOS["idt2b"],
        "utility_weights_degradation": {
            "accuracy":    DEGRADE_W_ACC,
            "latency":     DEGRADE_W_LAT,
            "reliability": DEGRADE_W_REL,
        },
        "utility_threshold":           UTILITY_FAILOVER_THRESHOLD,
        "qos_switch_hysteresis":       QOS_SWITCH_HYSTERESIS,
        "availability_threshold":      AVAILABILITY_FAILOVER_THRESHOLD,
        "sim_duration_s":              SIM_DURATION_S,
        "recovery_duration_s":         RECOVERY_DURATION_S,
        "degrade_rate_range":          [DEGRADE_RATE_MIN, DEGRADE_RATE_MAX],
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

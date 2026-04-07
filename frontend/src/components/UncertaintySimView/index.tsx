/**
 * UncertaintySimView — Uncertainty-Aware Selection Simulation Tab
 *
 * Evaluates three selection policies under degradation and measurement noise:
 *
 *   1. Baseline          — availability-based: switches when reliability < threshold
 *   2. QoS-aware         — deterministic utility-based selection
 *   3. Uncertainty-aware — risk-adjusted utility with separate quality (σ_q)
 *                          and availability (σ_a) uncertainty terms
 *
 * Three scenario presets expose the value of uncertainty modelling:
 *   • Scenario 1 (Limited)  — P2 ≈ P3
 *   • Scenario 2 (Clear)    — P3 begins to diverge
 *   • Scenario 3 (Extreme)  — P3 clearly prefers the stable provider
 *
 * All computation runs in-browser (no backend required).
 * For offline publication figures run: python scripts/uncertainty_sim.py
 */

import React, { useState, useCallback } from 'react'
import {
  ComposedChart, Line, Area, XAxis, YAxis,
  CartesianGrid, Tooltip, ReferenceLine, ResponsiveContainer, Legend,
  BarChart, Bar, Cell,
} from 'recharts'

// ─── Types ─────────────────────────────────────────────────────────────────────

interface ProvNom {
  accMean: number; latMean: number; relMean: number
  sigmaQ: number; sigmaA: number
}

interface UncScenarioConfig {
  key: string
  label: string
  tagColor: string
  tagBg: string
  description: string
  highlights: string[]
  // Providers
  provA: ProvNom
  provB: ProvNom
  // Policy parameters (wRel = 1 − wAcc − wLat, derived at use)
  wAcc: number; wLat: number
  riskQ: number; riskA: number
  availThreshold: number
  utilThreshold: number
  hysteresis: number
  // Simulation
  simDuration: number
  recoveryS: number
  episodeCount: number
  degradeRate: [number, number]
  onsetRange: [number, number]
  degradeWindow: [number, number]
  failWindow: [number, number]
  interGap: [number, number]
}

interface UncSimParams {
  runs: number; seed: number
  provAAccMean: number; provALatMean: number; provARelMean: number
  provASigmaQ: number;  provASigmaA: number
  provBAccMean: number; provBLatMean: number; provBRelMean: number
  provBSigmaQ: number;  provBSigmaA: number
  wAcc: number; wLat: number
  riskQ: number; riskA: number
  availThreshold: number; utilThreshold: number; hysteresis: number
  simDuration: number; recoveryS: number; episodeCount: number
  degradeRateMin: number; degradeRateMax: number
  onsetMin: number; onsetMax: number
  degradeWinMin: number; degradeWinMax: number
  failWinMin: number; failWinMax: number
  interGapMin: number; interGapMax: number
}

interface UncSimChartRow {
  t: number
  p1_med: number; p1_p10: number; p1_p90: number   // baseline utility
  p2_med: number; p2_p10: number; p2_p90: number   // qos_aware utility
  p3_med: number; p3_p10: number; p3_p90: number   // uncertainty_aware utility
  sel1_med: number   // 0=A, 1=B fraction
  sel2_med: number
  sel3_med: number
  diff21_med: number; diff21_p10: number; diff21_p90: number  // P2−P1
  diff32_med: number; diff32_p10: number; diff32_p90: number  // P3−P2
  diff31_med: number; diff31_p10: number; diff31_p90: number  // P3−P1
  sqA_med: number; sqB_med: number  // quality uncertainty
  saA_med: number; saB_med: number  // availability uncertainty
}

interface UncSimResult {
  rows: UncSimChartRow[]
  meanUtilBaseline: number
  meanUtilQoS: number
  meanUtilUncert: number
  nRuns: number
}

// ─── Scenario presets ──────────────────────────────────────────────────────────

const UNCERT_SCENARIOS: Record<string, UncScenarioConfig> = {
  scenario_1: {
    key: 'scenario_1', label: 'Scenario 1 — Limited Uncertainty',
    tagColor: '#374151', tagBg: '#e5e7eb',
    description:
      'Small measurement uncertainty (σ_q=0.02, σ_a=0.02) for both providers. '
      + 'Risk adjustment barely changes the provider rankings, so the QoS-aware and '
      + 'uncertainty-aware policies behave similarly. The baseline is still visibly '
      + 'weaker during degradation.',
    highlights: [
      'P2 and P3 mostly agree — uncertainty is too small to shift rankings',
      'Baseline switches only on hard failure; P2 and P3 switch proactively',
      'Difference plot P3−P2 stays near zero throughout',
    ],
    provA: { accMean: 0.92, latMean: 35, relMean: 0.95, sigmaQ: 0.02, sigmaA: 0.02 },
    provB: { accMean: 0.75, latMean: 10, relMean: 0.87, sigmaQ: 0.015, sigmaA: 0.015 },
    wAcc: 0.45, wLat: 0.20,  // wRel derived as 1 − wAcc − wLat = 0.35
    riskQ: 1.0, riskA: 1.0,
    availThreshold: 0.15, utilThreshold: 0.55, hysteresis: 0.06,
    simDuration: 120, recoveryS: 10, episodeCount: 2,
    degradeRate: [0.04, 0.10], onsetRange: [15, 25],
    degradeWindow: [8, 12], failWindow: [6, 10], interGap: [12, 20],
  },

  scenario_2: {
    key: 'scenario_2', label: 'Scenario 2 — Clear Uncertainty',
    tagColor: '#1d4ed8', tagBg: '#dbeafe',
    description:
      'Provider A has significantly higher uncertainty (σ_q=0.08, σ_a=0.06) than '
      + 'provider B (σ=0.02). With risk aversion ρ=1.5, the uncertainty-aware policy '
      + 'already slightly prefers B at nominal QoS. During degradation the policies '
      + 'clearly diverge: P3 switches away from the noisy provider sooner.',
    highlights: [
      'At nominal QoS: P2 barely prefers A, P3 barely prefers B — policies diverge',
      'During degradation σ spikes; P3 reacts earlier and more decisively',
      'Difference plot P3−P2 shows a clear positive region during episodes',
    ],
    provA: { accMean: 0.92, latMean: 35, relMean: 0.95, sigmaQ: 0.08, sigmaA: 0.06 },
    provB: { accMean: 0.75, latMean: 10, relMean: 0.87, sigmaQ: 0.02, sigmaA: 0.02 },
    wAcc: 0.45, wLat: 0.20,
    riskQ: 1.5, riskA: 1.5,
    availThreshold: 0.15, utilThreshold: 0.55, hysteresis: 0.05,
    simDuration: 120, recoveryS: 10, episodeCount: 2,
    degradeRate: [0.05, 0.12], onsetRange: [15, 25],
    degradeWindow: [8, 14], failWindow: [6, 12], interGap: [12, 20],
  },

  scenario_3: {
    key: 'scenario_3', label: 'Scenario 3 — Extreme Uncertainty',
    tagColor: '#b91c1c', tagBg: '#fee2e2',
    description:
      'Very high uncertainty for provider A (σ_q=0.18, σ_a=0.12) with strong risk '
      + 'aversion (ρ=2.0). Provider A has better nominal QoS, but its risk-adjusted '
      + 'utility is substantially lower. P3 prefers B throughout; P2 sticks with A. '
      + 'Three degradation episodes make the benefit of uncertainty modelling unmistakable.',
    highlights: [
      'P2 nominal utility: A=0.895, B=0.818 → P2 prefers A',
      'P3 risk-adj utility: A=0.649, B=0.786 → P3 strongly prefers B',
      'P3 outperforms P2 throughout — uncertainty modelling is clearly beneficial',
    ],
    provA: { accMean: 0.93, latMean: 30, relMean: 0.96, sigmaQ: 0.18, sigmaA: 0.12 },
    provB: { accMean: 0.74, latMean:  8, relMean: 0.86, sigmaQ: 0.02, sigmaA: 0.02 },
    wAcc: 0.45, wLat: 0.20,
    riskQ: 2.0, riskA: 2.0,
    availThreshold: 0.15, utilThreshold: 0.50, hysteresis: 0.04,
    simDuration: 150, recoveryS: 10, episodeCount: 3,
    degradeRate: [0.06, 0.15], onsetRange: [15, 25],
    degradeWindow: [8, 14], failWindow: [6, 12], interGap: [10, 18],
  },
}

// ─── Parameter initialisation ──────────────────────────────────────────────────

function paramsFromScenario(sc: UncScenarioConfig, runs = 30, seed = 1234): UncSimParams {
  return {
    runs, seed,
    provAAccMean: sc.provA.accMean, provALatMean: sc.provA.latMean,
    provARelMean: sc.provA.relMean, provASigmaQ: sc.provA.sigmaQ,
    provASigmaA: sc.provA.sigmaA,
    provBAccMean: sc.provB.accMean, provBLatMean: sc.provB.latMean,
    provBRelMean: sc.provB.relMean, provBSigmaQ: sc.provB.sigmaQ,
    provBSigmaA: sc.provB.sigmaA,
    wAcc: sc.wAcc, wLat: sc.wLat,
    riskQ: sc.riskQ, riskA: sc.riskA,
    availThreshold: sc.availThreshold,
    utilThreshold: sc.utilThreshold,
    hysteresis: sc.hysteresis,
    simDuration: sc.simDuration, recoveryS: sc.recoveryS,
    episodeCount: sc.episodeCount,
    degradeRateMin: sc.degradeRate[0], degradeRateMax: sc.degradeRate[1],
    onsetMin: sc.onsetRange[0], onsetMax: sc.onsetRange[1],
    degradeWinMin: sc.degradeWindow[0], degradeWinMax: sc.degradeWindow[1],
    failWinMin: sc.failWindow[0], failWinMax: sc.failWindow[1],
    interGapMin: sc.interGap[0], interGapMax: sc.interGap[1],
  }
}

// ─── PRNG (mulberry32) ─────────────────────────────────────────────────────────

function makePRNG(seed: number) {
  let s = (seed >>> 0) || 1
  const next = (): number => {
    s = (s + 0x6D2B79F5) >>> 0
    let t = Math.imul(s ^ (s >>> 15), 1 | s)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 0x100000000
  }
  const uniform = (a: number, b: number) => a + next() * (b - a)
  const choice = <T,>(arr: T[]): T => arr[Math.floor(next() * arr.length)]
  // Box-Muller normal distribution
  const normal = (mean: number, std: number): number => {
    const u1 = Math.max(1e-10, next())
    const u2 = next()
    const z = Math.sqrt(-2 * Math.log(u1)) * Math.cos(2 * Math.PI * u2)
    return mean + std * z
  }
  return { next, uniform, choice, normal }
}

// ─── Statistics ────────────────────────────────────────────────────────────────

function stats(arr: number[]): [number, number, number] {
  const s = [...arr].sort((a, b) => a - b)
  const n = s.length
  if (n === 0) return [0, 0, 0]
  const med = n % 2 === 0 ? (s[n/2 - 1] + s[n/2]) / 2 : s[Math.floor(n/2)]
  const p10 = s[Math.max(0, Math.floor(n * 0.10))]
  const p90 = s[Math.min(n - 1, Math.floor(n * 0.90))]
  return [med, p10, p90]
}

// ─── Simulation engine ─────────────────────────────────────────────────────────

const MAX_LAT_MS = 100

function computeUtility(acc: number, lat: number, rel: number,
                         wA: number, wL: number, wR: number): number {
  return wA * acc + wL * (1 - Math.min(1, lat / MAX_LAT_MS)) + wR * rel
}

/**
 * Risk-adjusted utility for Policy 3.
 * Quality uncertainty (sigmaQ) penalises the accuracy term.
 * Availability uncertainty (sigmaA) penalises the reliability term.
 * These are treated independently — matching the separate sigma_q / sigma_a model.
 */
function computeRiskAdjustedUtility(
  acc: number, lat: number, rel: number,
  sigmaQ: number, sigmaA: number,
  riskQ: number, riskA: number,
  wA: number, wL: number, wR: number,
): number {
  const adjAcc = Math.max(0, acc - riskQ * sigmaQ)
  const adjRel = Math.max(0, rel - riskA * sigmaA)
  return wA * adjAcc + wL * (1 - Math.min(1, lat / MAX_LAT_MS)) + wR * adjRel
}

interface Episode { onset: number; rate: number; failAt: number; recoverAt: number }
interface QoS { acc: number; lat: number; rel: number }

/** Degradation intensity in [0, 1]: 0=healthy, 1=hard-fail */
function degradationFactor(t: number, ep: Episode): number {
  const recEnd = ep.recoverAt + 10 // recovery_s baked into episode
  if (t < ep.onset || t >= recEnd) return 0
  if (t >= ep.onset && t < ep.failAt)
    return Math.min(1, (t - ep.onset) / Math.max(1e-9, ep.failAt - ep.onset))
  if (t >= ep.failAt && t < ep.recoverAt) return 1
  return Math.max(0, 1 - (t - ep.recoverAt) / Math.max(1e-9, recEnd - ep.recoverAt))
}

function trueQosAt(t: number, nom: QoS, eps: Episode[], recS: number): QoS {
  for (const ep of eps) {
    const recEnd = ep.recoverAt + recS
    if (t >= ep.onset && t < ep.failAt) {
      const e = t - ep.onset
      return { acc: Math.max(0, nom.acc - e * ep.rate),
               lat: nom.lat * (1 + e * ep.rate * 2),
               rel: Math.max(0, nom.rel - e * ep.rate * 0.8) }
    }
    if (t >= ep.failAt && t < ep.recoverAt)
      return { acc: 0, lat: nom.lat * 5, rel: 0 }
    if (t >= ep.recoverAt && t < recEnd) {
      const frac = (t - ep.recoverAt) / Math.max(1e-9, recS)
      return { acc: nom.acc * frac, lat: nom.lat * (4 - 3 * frac), rel: nom.rel * frac }
    }
  }
  return { ...nom }
}

/** Dynamic sigma: grows by degradationFactor * amplifier during episodes */
function dynamicSigma(t: number, sigmaBase: number, eps: Episode[], amplifier: number): number {
  let maxDeg = 0
  for (const ep of eps) maxDeg = Math.max(maxDeg, degradationFactor(t, ep))
  return sigmaBase * (1 + amplifier * maxDeg)
}

const SIGMA_AMP_Q = 3.0
const SIGMA_AMP_A = 2.0

/** Run simulation for a single seed, return per-timestep per-policy data */
function runOnce(p: UncSimParams, runSeed: number): {
  utility: Record<string, number[]>
  selection: Record<string, number[]>
  sqA: number[]; sqB: number[]
  saA: number[]; saB: number[]
} {
  const rng = makePRNG(runSeed)

  // Build episode lists
  const eps: Record<string, Episode[]> = { A: [], B: [] }
  let curStart = 0
  let degraded = rng.choice(['A', 'B'] as const)

  for (let i = 0; i < p.episodeCount; i++) {
    const onset = i === 0
      ? rng.uniform(p.onsetMin, p.onsetMax)
      : curStart + rng.uniform(p.interGapMin, p.interGapMax)
    const rate     = rng.uniform(p.degradeRateMin, p.degradeRateMax)
    const failAt   = onset + rng.uniform(p.degradeWinMin, p.degradeWinMax)
    const recoverAt = failAt + rng.uniform(p.failWinMin, p.failWinMax)
    const ep: Episode = { onset, rate, failAt, recoverAt }

    eps[degraded].push(ep)
    curStart = recoverAt + p.recoveryS
    if (i < p.episodeCount - 1)
      degraded = degraded === 'A' ? 'B' : 'A'
  }

  const nomA: QoS = { acc: p.provAAccMean, lat: p.provALatMean, rel: p.provARelMean }
  const nomB: QoS = { acc: p.provBAccMean, lat: p.provBLatMean, rel: p.provBRelMean }

  const wA = p.wAcc; const wL = p.wLat; const wR = 1 - wA - wL

  // Active provider for each policy (start on A)
  let activeBase = 'A'; let activePol2 = 'A'; let activePol3 = 'A'

  const utility: Record<string, number[]>   = { baseline: [], qos_aware: [], uncertainty_aware: [] }
  const selection: Record<string, number[]> = { baseline: [], qos_aware: [], uncertainty_aware: [] }
  const sqA: number[] = []; const sqB: number[] = []
  const saA: number[] = []; const saB: number[] = []

  for (let t = 0; t <= p.simDuration; t++) {
    const trueA = trueQosAt(t, nomA, eps['A'], p.recoveryS)
    const trueB = trueQosAt(t, nomB, eps['B'], p.recoveryS)

    // Dynamic sigmas (grow with degradation)
    const sQA = dynamicSigma(t, p.provASigmaQ, eps['A'], SIGMA_AMP_Q)
    const sQB = dynamicSigma(t, p.provBSigmaQ, eps['B'], SIGMA_AMP_Q)
    const sAA = dynamicSigma(t, p.provASigmaA, eps['A'], SIGMA_AMP_A)
    const sAB = dynamicSigma(t, p.provBSigmaA, eps['B'], SIGMA_AMP_A)

    sqA.push(sQA); sqB.push(sQB); saA.push(sAA); saB.push(sAB)

    // Noisy measurements (all policies see the same noise realisation)
    const measA: QoS = {
      acc: Math.max(0, Math.min(1, rng.normal(trueA.acc, sQA))),
      lat: Math.max(0, rng.normal(trueA.lat, 0)),
      rel: Math.max(0, Math.min(1, rng.normal(trueA.rel, sAA))),
    }
    const measB: QoS = {
      acc: Math.max(0, Math.min(1, rng.normal(trueB.acc, sQB))),
      lat: Math.max(0, rng.normal(trueB.lat, 0)),
      rel: Math.max(0, Math.min(1, rng.normal(trueB.rel, sAB))),
    }

    const measQos = { A: measA, B: measB }
    const sigmaQ  = { A: sQA, B: sQB }
    const sigmaAv = { A: sAA, B: sAB }

    // ── Policy 1: baseline — availability-based ─────────────────────────
    if (measQos[activeBase as 'A' | 'B'].rel < p.availThreshold) {
      const alt = activeBase === 'A' ? 'B' : 'A'
      if (measQos[alt as 'A' | 'B'].rel >= p.availThreshold) activeBase = alt
    }

    // ── Policy 2: QoS-aware — deterministic utility ─────────────────────
    {
      const cur  = measQos[activePol2 as 'A' | 'B']
      const alt  = activePol2 === 'A' ? 'B' : 'A'
      const uCur = computeUtility(cur.acc, cur.lat, cur.rel, wA, wL, wR)
      const uAlt = computeUtility(measQos[alt as 'A' | 'B'].acc,
                                   measQos[alt as 'A' | 'B'].lat,
                                   measQos[alt as 'A' | 'B'].rel, wA, wL, wR)
      const shouldSwitch = uCur < p.utilThreshold || uAlt > uCur + p.hysteresis
      if (shouldSwitch && uAlt > uCur) activePol2 = alt
    }

    // ── Policy 3: uncertainty-aware — risk-adjusted utility ─────────────
    {
      const cur  = measQos[activePol3 as 'A' | 'B']
      const alt  = activePol3 === 'A' ? 'B' : 'A'
      const uCur = computeRiskAdjustedUtility(
        cur.acc, cur.lat, cur.rel,
        sigmaQ[activePol3 as 'A' | 'B'], sigmaAv[activePol3 as 'A' | 'B'],
        p.riskQ, p.riskA, wA, wL, wR)
      const altQ = measQos[alt as 'A' | 'B']
      const uAlt = computeRiskAdjustedUtility(
        altQ.acc, altQ.lat, altQ.rel,
        sigmaQ[alt as 'A' | 'B'], sigmaAv[alt as 'A' | 'B'],
        p.riskQ, p.riskA, wA, wL, wR)
      const shouldSwitch = uCur < p.utilThreshold || uAlt > uCur + p.hysteresis
      if (shouldSwitch && uAlt > uCur) activePol3 = alt
    }

    // Record TRUE utility of selected provider for each policy
    for (const [pol, sel] of [
      ['baseline', activeBase], ['qos_aware', activePol2], ['uncertainty_aware', activePol3],
    ] as [string, string][]) {
      const tq = sel === 'A' ? trueA : trueB
      utility[pol].push(computeUtility(tq.acc, tq.lat, tq.rel, wA, wL, wR))
      selection[pol].push(sel === 'B' ? 1 : 0)
    }
  }

  return { utility, selection, sqA, sqB, saA, saB }
}

/** Run N seeds and aggregate into chart rows */
function runSimulation(p: UncSimParams): UncSimResult {
  // Per-timestep per-policy collections for aggregation
  const utilByPol: Record<string, number[][]> = {
    baseline: [], qos_aware: [], uncertainty_aware: [],
  }
  const selByPol: Record<string, number[][]> = {
    baseline: [], qos_aware: [], uncertainty_aware: [],
  }
  const sqA: number[][] = []; const sqB: number[][] = []
  const saA: number[][] = []; const saB: number[][] = []

  for (let r = 0; r < p.runs; r++) {
    const res = runOnce(p, p.seed + r)
    for (const pol of ['baseline', 'qos_aware', 'uncertainty_aware']) {
      if (utilByPol[pol].length === 0) {
        // Initialise per-timestep arrays
        utilByPol[pol] = res.utility[pol].map(v => [v])
        selByPol[pol]  = res.selection[pol].map(v => [v])
      } else {
        res.utility[pol].forEach((v, i)   => utilByPol[pol][i].push(v))
        res.selection[pol].forEach((v, i) => selByPol[pol][i].push(v))
      }
    }
    if (sqA.length === 0) {
      res.sqA.forEach(v => sqA.push([v]))
      res.sqB.forEach(v => sqB.push([v]))
      res.saA.forEach(v => saA.push([v]))
      res.saB.forEach(v => saB.push([v]))
    } else {
      res.sqA.forEach((v, i) => sqA[i].push(v))
      res.sqB.forEach((v, i) => sqB[i].push(v))
      res.saA.forEach((v, i) => saA[i].push(v))
      res.saB.forEach((v, i) => saB[i].push(v))
    }
  }

  const n = p.simDuration + 1
  const rows: UncSimChartRow[] = []
  let sumU1 = 0; let sumU2 = 0; let sumU3 = 0; let total = 0

  for (let i = 0; i < n; i++) {
    const [m1, p1lo, p1hi] = stats(utilByPol.baseline[i] ?? [0])
    const [m2, p2lo, p2hi] = stats(utilByPol.qos_aware[i] ?? [0])
    const [m3, p3lo, p3hi] = stats(utilByPol.uncertainty_aware[i] ?? [0])

    const sel1 = (selByPol.baseline[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs
    const sel2 = (selByPol.qos_aware[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs
    const sel3 = (selByPol.uncertainty_aware[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs

    // Differences (paired: sort both arrays before subtraction)
    const s1 = [...(utilByPol.baseline[i] ?? [0])].sort((a, b) => a - b)
    const s2 = [...(utilByPol.qos_aware[i] ?? [0])].sort((a, b) => a - b)
    const s3 = [...(utilByPol.uncertainty_aware[i] ?? [0])].sort((a, b) => a - b)

    const d21 = s2.map((v, j) => v - (s1[j] ?? 0))
    const d32 = s3.map((v, j) => v - (s2[j] ?? 0))
    const d31 = s3.map((v, j) => v - (s1[j] ?? 0))

    const [d21m, d21lo, d21hi] = stats(d21)
    const [d32m, d32lo, d32hi] = stats(d32)
    const [d31m, d31lo, d31hi] = stats(d31)

    const sqAm = (sqA[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs
    const sqBm = (sqB[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs
    const saAm = (saA[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs
    const saBm = (saB[i] ?? [0]).reduce((a, b) => a + b, 0) / p.runs

    rows.push({
      t: i,
      p1_med: m1, p1_p10: p1lo, p1_p90: p1hi,
      p2_med: m2, p2_p10: p2lo, p2_p90: p2hi,
      p3_med: m3, p3_p10: p3lo, p3_p90: p3hi,
      sel1_med: sel1, sel2_med: sel2, sel3_med: sel3,
      diff21_med: d21m, diff21_p10: d21lo, diff21_p90: d21hi,
      diff32_med: d32m, diff32_p10: d32lo, diff32_p90: d32hi,
      diff31_med: d31m, diff31_p10: d31lo, diff31_p90: d31hi,
      sqA_med: sqAm, sqB_med: sqBm,
      saA_med: saAm, saB_med: saBm,
    })

    sumU1 += m1; sumU2 += m2; sumU3 += m3; total++
  }

  return {
    rows,
    meanUtilBaseline: sumU1 / total,
    meanUtilQoS:      sumU2 / total,
    meanUtilUncert:   sumU3 / total,
    nRuns: p.runs,
  }
}

// ─── Colours ───────────────────────────────────────────────────────────────────

const C_BASE   = '#b91c1c'   // dark red     — baseline
const C_QOS    = '#1d4ed8'   // dark blue    — QoS-aware
const C_UNCERT = '#15803d'   // dark green   — uncertainty-aware
const C_PROV_A = '#7c3aed'   // purple       — provider A
const C_PROV_B = '#d97706'   // amber        — provider B

// ─── Shared chart helpers ──────────────────────────────────────────────────────

const chartStyle: React.CSSProperties = {
  background: 'var(--bg-elevated)',
  borderRadius: 8,
  border: '1px solid var(--border)',
  padding: '16px 8px 12px',
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ fontWeight: 700, fontSize: '0.82rem', color: 'var(--text-primary)',
                  marginBottom: 12, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
      {children}
    </div>
  )
}

function ChartLabel({ children }: { children: React.ReactNode }) {
  return (
    <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)', marginBottom: 4 }}>
      {children}
    </div>
  )
}

// ─── Individual chart panels ───────────────────────────────────────────────────

function UtilityChart({ rows }: { rows: UncSimChartRow[] }) {
  return (
    <div style={chartStyle}>
      <SectionTitle>Utility Over Time</SectionTitle>
      <ChartLabel>True utility of the selected provider for each policy. Shaded band: 10th–90th percentile across runs.</ChartLabel>
      <ResponsiveContainer width="100%" height={260}>
        <ComposedChart data={rows} margin={{ top: 4, right: 24, bottom: 4, left: 8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="t" tick={{ fontSize: 10 }} label={{ value: 'Simulation time (s)', position: 'insideBottomRight', offset: -4, fontSize: 10 }} />
          <YAxis domain={[-0.02, 1.10]} tick={{ fontSize: 10 }} label={{ value: 'Utility', angle: -90, position: 'insideLeft', offset: 8, fontSize: 10 }} />
          <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(3) : v} contentStyle={{ fontSize: 11 }} />
          <Area dataKey="p1_p90" fill={C_BASE}   stroke="none" fillOpacity={0.08} legendType="none" />
          <Area dataKey="p1_p10" fill="white"    stroke="none" fillOpacity={1.0}  legendType="none" />
          <Area dataKey="p2_p90" fill={C_QOS}    stroke="none" fillOpacity={0.08} legendType="none" />
          <Area dataKey="p2_p10" fill="white"    stroke="none" fillOpacity={1.0}  legendType="none" />
          <Area dataKey="p3_p90" fill={C_UNCERT} stroke="none" fillOpacity={0.10} legendType="none" />
          <Area dataKey="p3_p10" fill="white"    stroke="none" fillOpacity={1.0}  legendType="none" />
          <Line dataKey="p1_med" stroke={C_BASE}   dot={false} strokeWidth={2} strokeDasharray="5 3" name="Baseline" />
          <Line dataKey="p2_med" stroke={C_QOS}    dot={false} strokeWidth={2} name="QoS-aware" />
          <Line dataKey="p3_med" stroke={C_UNCERT} dot={false} strokeWidth={2.5} name="Uncertainty-aware" />
          <Legend wrapperStyle={{ fontSize: 11 }} />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

function SelectionChart({ rows }: { rows: UncSimChartRow[] }) {
  return (
    <div style={chartStyle}>
      <SectionTitle>Provider Selection Over Time</SectionTitle>
      <ChartLabel>Fraction of runs selecting provider B. 1.0 = all runs chose B; 0.0 = all chose A.</ChartLabel>
      <ResponsiveContainer width="100%" height={200}>
        <ComposedChart data={rows} margin={{ top: 4, right: 24, bottom: 4, left: 8 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="t" tick={{ fontSize: 10 }} />
          <YAxis domain={[0, 1.05]} tick={{ fontSize: 10 }} label={{ value: 'Frac. using B', angle: -90, position: 'insideLeft', offset: 8, fontSize: 10 }} />
          <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(2) : v} contentStyle={{ fontSize: 11 }} />
          <ReferenceLine y={0.5} stroke="#9ca3af" strokeDasharray="4 2" />
          <Line dataKey="sel1_med" stroke={C_BASE}   dot={false} strokeWidth={1.5} strokeDasharray="5 3" name="Baseline" />
          <Line dataKey="sel2_med" stroke={C_QOS}    dot={false} strokeWidth={1.5} name="QoS-aware" />
          <Line dataKey="sel3_med" stroke={C_UNCERT} dot={false} strokeWidth={2} name="Uncertainty-aware" />
          <Legend wrapperStyle={{ fontSize: 11 }} />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

function DifferenceChart({ rows }: { rows: UncSimChartRow[] }) {
  return (
    <div style={chartStyle}>
      <SectionTitle>Difference Plots</SectionTitle>
      <ChartLabel>Median utility advantage (positive = first policy outperforms second). Shaded band: p10–p90.</ChartLabel>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 12 }}>
        {[
          { med: 'diff21_med', lo: 'diff21_p10', hi: 'diff21_p90', color: C_QOS,    label: 'QoS-aware − Baseline  (P2 − P1)' },
          { med: 'diff32_med', lo: 'diff32_p10', hi: 'diff32_p90', color: C_UNCERT, label: 'Uncertainty-aware − QoS-aware  (P3 − P2)' },
          { med: 'diff31_med', lo: 'diff31_p10', hi: 'diff31_p90', color: '#7c3aed', label: 'Uncertainty-aware − Baseline  (P3 − P1)' },
        ].map(({ med, lo, hi, color, label }) => (
          <div key={med}>
            <div style={{ fontSize: '0.70rem', color: 'var(--text-muted)', marginBottom: 2 }}>{label}</div>
            <ResponsiveContainer width="100%" height={120}>
              <ComposedChart data={rows} margin={{ top: 2, right: 24, bottom: 2, left: 8 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                <XAxis dataKey="t" tick={{ fontSize: 9 }} />
                <YAxis tick={{ fontSize: 9 }} tickFormatter={(v: number) => v.toFixed(2)} />
                <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(3) : v} contentStyle={{ fontSize: 10 }} />
                <ReferenceLine y={0} stroke="#374151" strokeDasharray="4 2" />
                <Area dataKey={hi} fill={color} stroke="none" fillOpacity={0.12} legendType="none" />
                <Area dataKey={lo} fill="white" stroke="none" fillOpacity={1.0}  legendType="none" />
                <Line dataKey={med} stroke={color} dot={false} strokeWidth={2} />
              </ComposedChart>
            </ResponsiveContainer>
          </div>
        ))}
      </div>
    </div>
  )
}

function UncertaintyChart({ rows }: { rows: UncSimChartRow[] }) {
  return (
    <div style={chartStyle}>
      <SectionTitle>Uncertainty Evolution Over Time</SectionTitle>
      <ChartLabel>Dynamic σ values grow during degradation episodes (σ_q spikes for the degrading provider).</ChartLabel>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        <div>
          <div style={{ fontSize: '0.70rem', color: 'var(--text-muted)', marginBottom: 2 }}>Quality uncertainty σ_q</div>
          <ResponsiveContainer width="100%" height={160}>
            <ComposedChart data={rows} margin={{ top: 2, right: 16, bottom: 2, left: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="t" tick={{ fontSize: 9 }} />
              <YAxis tick={{ fontSize: 9 }} tickFormatter={(v: number) => v.toFixed(3)} />
              <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(4) : v} contentStyle={{ fontSize: 10 }} />
              <Line dataKey="sqA_med" stroke={C_PROV_A} dot={false} strokeWidth={2} name="Provider A" />
              <Line dataKey="sqB_med" stroke={C_PROV_B} dot={false} strokeWidth={2} strokeDasharray="4 2" name="Provider B" />
              <Legend wrapperStyle={{ fontSize: 10 }} />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
        <div>
          <div style={{ fontSize: '0.70rem', color: 'var(--text-muted)', marginBottom: 2 }}>Availability uncertainty σ_a</div>
          <ResponsiveContainer width="100%" height={160}>
            <ComposedChart data={rows} margin={{ top: 2, right: 16, bottom: 2, left: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="t" tick={{ fontSize: 9 }} />
              <YAxis tick={{ fontSize: 9 }} tickFormatter={(v: number) => v.toFixed(3)} />
              <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(4) : v} contentStyle={{ fontSize: 10 }} />
              <Line dataKey="saA_med" stroke={C_PROV_A} dot={false} strokeWidth={2} name="Provider A" />
              <Line dataKey="saB_med" stroke={C_PROV_B} dot={false} strokeWidth={2} strokeDasharray="4 2" name="Provider B" />
              <Legend wrapperStyle={{ fontSize: 10 }} />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

function SummaryChart({ result, params }: { result: UncSimResult; params: UncSimParams }) {
  const wA = params.wAcc; const wL = params.wLat; const wR = 1 - wA - wL
  const nomUtil = (acc: number, lat: number, rel: number) =>
    computeUtility(acc, lat, rel, wA, wL, wR)
  const adjUtil = (acc: number, lat: number, rel: number,
                   sQ: number, sA: number) =>
    computeRiskAdjustedUtility(acc, lat, rel, sQ, sA, params.riskQ, params.riskA, wA, wL, wR)

  const barData = [
    { name: 'Baseline', util: result.meanUtilBaseline, color: C_BASE },
    { name: 'QoS-aware', util: result.meanUtilQoS,     color: C_QOS },
    { name: 'Uncert.-aware', util: result.meanUtilUncert, color: C_UNCERT },
  ]
  const scoreData = [
    { name: 'P2: A (nom.)', score: nomUtil(params.provAAccMean, params.provALatMean, params.provARelMean), color: C_PROV_A },
    { name: 'P2: B (nom.)', score: nomUtil(params.provBAccMean, params.provBLatMean, params.provBRelMean), color: C_PROV_B },
    { name: 'P3: A (adj.)', score: adjUtil(params.provAAccMean, params.provALatMean, params.provARelMean, params.provASigmaQ, params.provASigmaA), color: C_PROV_A },
    { name: 'P3: B (adj.)', score: adjUtil(params.provBAccMean, params.provBLatMean, params.provBRelMean, params.provBSigmaQ, params.provBSigmaA), color: C_PROV_B },
  ]

  return (
    <div style={chartStyle}>
      <SectionTitle>Summary</SectionTitle>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        <div>
          <ChartLabel>Mean true utility per policy (full run, all {result.nRuns} seeds)</ChartLabel>
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={barData} margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="name" tick={{ fontSize: 10 }} />
              <YAxis domain={[0, 1]} tick={{ fontSize: 10 }} />
              <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(4) : v} contentStyle={{ fontSize: 10 }} />
              <Bar dataKey="util">
                {barData.map((d, i) => <Cell key={i} fill={d.color} />)}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div>
          <ChartLabel>Provider scores at nominal QoS: deterministic (P2) vs risk-adjusted (P3)</ChartLabel>
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={scoreData} margin={{ top: 4, right: 16, bottom: 4, left: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis dataKey="name" tick={{ fontSize: 9 }} />
              <YAxis domain={[0, 1]} tick={{ fontSize: 10 }} />
              <Tooltip formatter={(v: any) => typeof v === 'number' ? v.toFixed(4) : v} contentStyle={{ fontSize: 10 }} />
              <Bar dataKey="score">
                {scoreData.map((d, i) => <Cell key={i} fill={d.color} opacity={i >= 2 ? 0.65 : 1} />)}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>
    </div>
  )
}

// ─── Parameter field helper ────────────────────────────────────────────────────

function Field({
  label, value, onChange, min, max, step = 0.01, fullWidth = false,
}: {
  label: string; value: number; onChange: (v: number) => void
  min: number; max: number; step?: number; fullWidth?: boolean
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 2,
                  gridColumn: fullWidth ? '1 / -1' : undefined }}>
      <label style={{ fontSize: '0.68rem', color: 'var(--text-muted)',
                      textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </label>
      <input
        type="number" value={value} min={min} max={max} step={step}
        onChange={e => onChange(parseFloat(e.target.value) || 0)}
        style={{
          width: '100%', padding: '4px 6px', fontSize: '0.78rem',
          borderRadius: 5, border: '1px solid var(--border)',
          background: 'var(--bg-elevated)', color: 'var(--text-primary)',
        }}
      />
    </div>
  )
}

// ─── Main component ────────────────────────────────────────────────────────────

const UncertaintySimView: React.FC = () => {
  const [selectedScenario, setSelectedScenario] = useState<string>('scenario_1')
  const [params, setParams]     = useState<UncSimParams>(() =>
    paramsFromScenario(UNCERT_SCENARIOS.scenario_1))
  const [result, setResult]     = useState<UncSimResult | null>(null)
  const [running, setRunning]   = useState(false)
  const [showParams, setShowParams] = useState(true)

  const scenarioCfg = UNCERT_SCENARIOS[selectedScenario]

  const handleScenarioChange = useCallback((key: string) => {
    setSelectedScenario(key)
    setParams(paramsFromScenario(UNCERT_SCENARIOS[key]))
    setResult(null)
  }, [])

  const handleReset = useCallback(() => {
    setParams(paramsFromScenario(scenarioCfg))
    setResult(null)
  }, [scenarioCfg])

  const setParam = useCallback(<K extends keyof UncSimParams>(key: K, val: UncSimParams[K]) => {
    setParams(p => ({ ...p, [key]: val }))
  }, [])

  const handleRun = useCallback(() => {
    setRunning(true)
    // Use setTimeout to allow React to re-render the button state before heavy computation
    setTimeout(() => {
      try {
        const r = runSimulation(params)
        setResult(r)
      } finally {
        setRunning(false)
      }
    }, 20)
  }, [params])

  const wRel = (1 - params.wAcc - params.wLat).toFixed(2)

  return (
    <div style={{ padding: '24px 28px', maxWidth: 1400, margin: '0 auto' }}>

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <div style={{ marginBottom: 20 }}>
        <div style={{ fontWeight: 800, fontSize: '1.15rem', color: 'var(--text-primary)',
                      letterSpacing: '-0.02em', marginBottom: 4 }}>
          Uncertainty-Aware Selection Simulation
        </div>
        <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', maxWidth: 800 }}>
          Compares three selection policies — availability-based baseline, deterministic QoS-aware,
          and risk-adjusted uncertainty-aware — across three scenario presets. All computation runs
          in-browser. For publication-quality figures run:{' '}
          <code style={{ fontFamily: 'monospace', fontSize: '0.75rem',
                         background: 'var(--bg-elevated)', padding: '1px 4px', borderRadius: 3 }}>
            python scripts/uncertainty_sim.py
          </code>
        </div>
      </div>

      {/* ── Scenario selector ─────────────────────────────────────────────── */}
      <div style={{ display: 'flex', gap: 10, marginBottom: 20, flexWrap: 'wrap' }}>
        {Object.values(UNCERT_SCENARIOS).map(sc => (
          <button
            key={sc.key}
            onClick={() => handleScenarioChange(sc.key)}
            style={{
              padding: '8px 14px', borderRadius: 8, cursor: 'pointer',
              border: selectedScenario === sc.key ? `2px solid ${sc.tagColor}` : '1px solid var(--border)',
              background: selectedScenario === sc.key ? sc.tagBg : 'var(--bg-surface)',
              color: selectedScenario === sc.key ? sc.tagColor : 'var(--text-secondary)',
              fontWeight: selectedScenario === sc.key ? 700 : 400,
              fontSize: '0.78rem', transition: 'all 120ms ease',
            }}
          >
            {sc.label}
          </button>
        ))}
      </div>

      {/* ── Scenario description ───────────────────────────────────────────── */}
      <div style={{
        background: 'var(--bg-elevated)', border: `1px solid ${scenarioCfg.tagColor}40`,
        borderRadius: 8, padding: '14px 18px', marginBottom: 20,
      }}>
        <div style={{ fontWeight: 700, fontSize: '0.82rem', color: scenarioCfg.tagColor,
                      marginBottom: 6 }}>
          {scenarioCfg.label}
        </div>
        <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', marginBottom: 8 }}>
          {scenarioCfg.description}
        </div>
        <ul style={{ margin: 0, paddingLeft: '1.4em',
                     fontSize: '0.75rem', color: 'var(--text-muted)' }}>
          {scenarioCfg.highlights.map((h, i) => <li key={i}>{h}</li>)}
        </ul>
      </div>

      {/* ── Controls bar ──────────────────────────────────────────────────── */}
      <div style={{ display: 'flex', gap: 10, marginBottom: 16, alignItems: 'center',
                    flexWrap: 'wrap' }}>
        <button
          onClick={handleRun}
          disabled={running}
          style={{
            padding: '8px 20px', borderRadius: 7, border: 'none', cursor: 'pointer',
            background: running ? 'var(--text-muted)' : 'var(--blue)',
            color: '#fff', fontWeight: 700, fontSize: '0.82rem',
          }}
        >
          {running ? 'Running…' : 'Run Simulation'}
        </button>
        <button
          onClick={handleReset}
          style={{
            padding: '8px 14px', borderRadius: 7, cursor: 'pointer',
            border: '1px solid var(--border)', background: 'var(--bg-surface)',
            color: 'var(--text-secondary)', fontSize: '0.78rem',
          }}
        >
          Reset to Defaults
        </button>
        <button
          onClick={() => setShowParams(v => !v)}
          style={{
            padding: '8px 14px', borderRadius: 7, cursor: 'pointer',
            border: '1px solid var(--border)',
            background: showParams ? 'var(--bg-elevated)' : 'var(--bg-surface)',
            color: 'var(--text-secondary)', fontSize: '0.78rem',
          }}
        >
          {showParams ? 'Hide Parameters' : 'Edit Parameters'}
        </button>
        <div style={{ fontSize: '0.74rem', color: 'var(--text-muted)', marginLeft: 4 }}>
          {params.runs} runs · seed {params.seed} · {params.simDuration}s
        </div>
      </div>

      {/* ── Parameter editor ──────────────────────────────────────────────── */}
      {showParams && (
        <div style={{
          background: 'var(--bg-surface)', border: '1px solid var(--border)',
          borderRadius: 10, padding: '18px 20px', marginBottom: 24,
        }}>
          <div style={{ fontWeight: 700, fontSize: '0.82rem', color: 'var(--text-primary)',
                        marginBottom: 14, display: 'flex', alignItems: 'center', gap: 8 }}>
            Simulation Parameters
            <span style={{ fontSize: '0.70rem', color: 'var(--text-muted)', fontWeight: 400 }}>
              (changes take effect on next Run)
            </span>
          </div>

          {/* Provider A */}
          <div style={{ marginBottom: 14 }}>
            <div style={{ fontSize: '0.72rem', fontWeight: 600, color: C_PROV_A,
                          marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Provider A — Quality Sensor
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 10 }}>
              <Field label="Acc mean"   value={params.provAAccMean} min={0} max={1} step={0.01} onChange={v => setParam('provAAccMean', v)} />
              <Field label="Lat mean (ms)" value={params.provALatMean} min={1} max={100} step={1} onChange={v => setParam('provALatMean', v)} />
              <Field label="Rel mean"   value={params.provARelMean} min={0} max={1} step={0.01} onChange={v => setParam('provARelMean', v)} />
              <Field label="σ_q (quality)" value={params.provASigmaQ} min={0} max={0.5} step={0.005} onChange={v => setParam('provASigmaQ', v)} />
              <Field label="σ_a (avail.)"  value={params.provASigmaA} min={0} max={0.5} step={0.005} onChange={v => setParam('provASigmaA', v)} />
            </div>
          </div>

          {/* Provider B */}
          <div style={{ marginBottom: 14 }}>
            <div style={{ fontSize: '0.72rem', fontWeight: 600, color: C_PROV_B,
                          marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Provider B — Fast Sensor
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 10 }}>
              <Field label="Acc mean"   value={params.provBAccMean} min={0} max={1} step={0.01} onChange={v => setParam('provBAccMean', v)} />
              <Field label="Lat mean (ms)" value={params.provBLatMean} min={1} max={100} step={1} onChange={v => setParam('provBLatMean', v)} />
              <Field label="Rel mean"   value={params.provBRelMean} min={0} max={1} step={0.01} onChange={v => setParam('provBRelMean', v)} />
              <Field label="σ_q (quality)" value={params.provBSigmaQ} min={0} max={0.5} step={0.005} onChange={v => setParam('provBSigmaQ', v)} />
              <Field label="σ_a (avail.)"  value={params.provBSigmaA} min={0} max={0.5} step={0.005} onChange={v => setParam('provBSigmaA', v)} />
            </div>
          </div>

          {/* Policy weights */}
          <div style={{ marginBottom: 14 }}>
            <div style={{ fontSize: '0.72rem', fontWeight: 600, color: 'var(--text-secondary)',
                          marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              QoS Utility Weights  (w_rel = 1 − w_acc − w_lat = {wRel})
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10 }}>
              <Field label="w_acc"  value={params.wAcc}  min={0} max={1}   step={0.05} onChange={v => setParam('wAcc', v)} />
              <Field label="w_lat"  value={params.wLat}  min={0} max={1}   step={0.05} onChange={v => setParam('wLat', v)} />
              <Field label="ρ_q (risk acc)"  value={params.riskQ} min={0} max={5}   step={0.1} onChange={v => setParam('riskQ', v)} />
              <Field label="ρ_a (risk rel)"  value={params.riskA} min={0} max={5}   step={0.1} onChange={v => setParam('riskA', v)} />
              <Field label="Avail. threshold" value={params.availThreshold} min={0} max={1} step={0.01} onChange={v => setParam('availThreshold', v)} />
              <Field label="Util. threshold"  value={params.utilThreshold}  min={0} max={1} step={0.01} onChange={v => setParam('utilThreshold', v)} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10, marginTop: 10 }}>
              <Field label="Hysteresis" value={params.hysteresis} min={0} max={0.3} step={0.01} onChange={v => setParam('hysteresis', v)} />
            </div>
          </div>

          {/* Degradation */}
          <div style={{ marginBottom: 14 }}>
            <div style={{ fontSize: '0.72rem', fontWeight: 600, color: 'var(--text-secondary)',
                          marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Degradation
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10 }}>
              <Field label="Sim duration (s)" value={params.simDuration} min={30} max={600} step={10} onChange={v => setParam('simDuration', v)} />
              <Field label="Recovery (s)"      value={params.recoveryS}   min={1}  max={60}  step={1}  onChange={v => setParam('recoveryS', v)} />
              <Field label="Episodes"          value={params.episodeCount} min={1} max={6}   step={1}  onChange={v => setParam('episodeCount', v)} />
              <Field label="Degrade rate min"  value={params.degradeRateMin} min={0.01} max={0.5} step={0.01} onChange={v => setParam('degradeRateMin', v)} />
              <Field label="Degrade rate max"  value={params.degradeRateMax} min={0.01} max={0.5} step={0.01} onChange={v => setParam('degradeRateMax', v)} />
              <Field label="Onset min (s)"     value={params.onsetMin}   min={1}  max={100} step={1}  onChange={v => setParam('onsetMin', v)} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10, marginTop: 10 }}>
              <Field label="Onset max (s)"       value={params.onsetMax}     min={1}   max={100} step={1} onChange={v => setParam('onsetMax', v)} />
              <Field label="Degrade win min (s)"  value={params.degradeWinMin} min={1}  max={60}  step={1} onChange={v => setParam('degradeWinMin', v)} />
              <Field label="Degrade win max (s)"  value={params.degradeWinMax} min={1}  max={60}  step={1} onChange={v => setParam('degradeWinMax', v)} />
              <Field label="Fail win min (s)"     value={params.failWinMin}   min={1}  max={60}  step={1} onChange={v => setParam('failWinMin', v)} />
              <Field label="Fail win max (s)"     value={params.failWinMax}   min={1}  max={60}  step={1} onChange={v => setParam('failWinMax', v)} />
              <Field label="Inter-gap min (s)"    value={params.interGapMin}  min={1}  max={60}  step={1} onChange={v => setParam('interGapMin', v)} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10, marginTop: 10 }}>
              <Field label="Inter-gap max (s)"  value={params.interGapMax} min={1} max={60} step={1} onChange={v => setParam('interGapMax', v)} />
              <Field label="Runs"  value={params.runs} min={1} max={200} step={1} onChange={v => setParam('runs', v)} />
              <Field label="Seed"  value={params.seed} min={0} max={99999} step={1} onChange={v => setParam('seed', v)} />
            </div>
          </div>
        </div>
      )}

      {/* ── Results ───────────────────────────────────────────────────────── */}
      {result === null && !running && (
        <div style={{
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          borderRadius: 10, padding: '32px', textAlign: 'center',
          color: 'var(--text-muted)', fontSize: '0.82rem',
        }}>
          Press <strong>Run Simulation</strong> to compute results for {scenarioCfg.label}.
        </div>
      )}

      {running && (
        <div style={{
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          borderRadius: 10, padding: '32px', textAlign: 'center',
          color: 'var(--text-secondary)', fontSize: '0.82rem',
        }}>
          <span className="loading-spinner" style={{ marginRight: 8 }} />
          Running {params.runs} simulation runs…
        </div>
      )}

      {result && !running && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
          {/* Outcome summary pills */}
          <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
            {[
              { label: 'Baseline', val: result.meanUtilBaseline, color: C_BASE },
              { label: 'QoS-aware', val: result.meanUtilQoS, color: C_QOS },
              { label: 'Uncertainty-aware', val: result.meanUtilUncert, color: C_UNCERT },
            ].map(({ label, val, color }) => (
              <div key={label} style={{
                background: 'var(--bg-elevated)', border: `1px solid ${color}60`,
                borderRadius: 8, padding: '10px 18px', minWidth: 180,
              }}>
                <div style={{ fontSize: '0.70rem', color: 'var(--text-muted)',
                              textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 4 }}>
                  {label}
                </div>
                <div style={{ fontSize: '1.3rem', fontWeight: 800, color, letterSpacing: '-0.02em' }}>
                  {val.toFixed(4)}
                </div>
                <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', marginTop: 2 }}>
                  mean true utility
                </div>
              </div>
            ))}
            <div style={{
              background: 'var(--bg-elevated)', border: '1px solid var(--border)',
              borderRadius: 8, padding: '10px 18px', minWidth: 200,
            }}>
              <div style={{ fontSize: '0.70rem', color: 'var(--text-muted)',
                            textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 4 }}>
                Advantage (P3 − P1)
              </div>
              <div style={{ fontSize: '1.3rem', fontWeight: 800, color: C_UNCERT, letterSpacing: '-0.02em' }}>
                {(result.meanUtilUncert - result.meanUtilBaseline >= 0 ? '+' : '')}
                {(result.meanUtilUncert - result.meanUtilBaseline).toFixed(4)}
              </div>
              <div style={{ fontSize: '0.68rem', color: 'var(--text-muted)', marginTop: 2 }}>
                uncertainty-aware vs baseline
              </div>
            </div>
          </div>

          {/* Charts */}
          <UtilityChart rows={result.rows} />
          <SelectionChart rows={result.rows} />
          <DifferenceChart rows={result.rows} />
          <UncertaintyChart rows={result.rows} />
          <SummaryChart result={result} params={params} />
        </div>
      )}
    </div>
  )
}

export default UncertaintySimView

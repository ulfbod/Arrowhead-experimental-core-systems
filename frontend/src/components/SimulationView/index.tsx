import React, { useState, useEffect, useRef, useCallback } from 'react'
import {
  ComposedChart, Line, Area, XAxis, YAxis,
  CartesianGrid, Tooltip, ReferenceLine, ResponsiveContainer,
} from 'recharts'

// ─── Scenario configurations (mirrors experiments.py SCENARIO_CONFIGS) ────────

interface ProviderRange { acc: [number, number]; lat: [number, number]; rel: [number, number] }
interface ScenarioConfig {
  label: string
  tagColor: string
  tagBg: string
  description: string
  steps: string[]
  demonstrates: string[]
  // tradeoff
  alphaSteps: number
  provA: ProviderRange
  provB: ProviderRange
  // degradation
  simDuration: number
  recoveryDuration: number
  episodeCount: number
  degradeRate: [number, number]
  onsetRange: [number, number]
  degradeWindow: [number, number]
  failWindow: [number, number]
  interGap: [number, number]
  nomA: { acc: number; lat: number; rel: number }
  nomB: { acc: number; lat: number; rel: number }
  wAcc: number; wLat: number; wRel: number
  utilThreshold: number
  hysteresis: number
  availThreshold: number
}

const SCENARIOS: Record<string, ScenarioConfig> = {
  basic: {
    label: 'basic', tagColor: '#374151', tagBg: '#e5e7eb',
    description:
      'Standard evaluation with moderate provider separation. Two alternating degradation episodes '
      + '(120 s) demonstrate the fundamental advantage of QoS-aware selection over pure '
      + 'availability-based switching during gradual degradation and recovery.',
    steps: [
      'Provider profiles are drawn from slightly overlapping ranges (A: high acc/slow, B: low acc/fast)',
      'Episode 1 degrades one provider; QoS-aware switches early, baseline waits for hard failure',
      'Provider recovers; QoS-aware switches back (hysteresis=0.06), baseline stays on fallback',
      'Episode 2 repeats on the other provider — showing both switch and switch-back behaviour',
    ],
    demonstrates: [
      'Basic utility-driven provider selection vs. availability-only selection',
      'Proactive switch-back after recovery',
    ],
    alphaSteps: 21,
    provA: { acc: [0.88, 0.99], lat: [40, 80], rel: [0.90, 0.99] },
    provB: { acc: [0.50, 0.74], lat: [3, 15],  rel: [0.68, 0.87] },
    simDuration: 120, recoveryDuration: 10, episodeCount: 2,
    degradeRate: [0.04, 0.15],
    onsetRange: [10, 18], degradeWindow: [8, 14], failWindow: [6, 10], interGap: [6, 14],
    nomA: { acc: 0.97, lat: 20, rel: 0.99 },
    nomB: { acc: 0.75, lat: 8,  rel: 0.91 },
    wAcc: 0.40, wLat: 0.30, wRel: 0.30,
    utilThreshold: 0.55, hysteresis: 0.06, availThreshold: 0.15,
  },
  improved01: {
    label: 'improved01', tagColor: '#1d4ed8', tagBg: '#dbeafe',
    description:
      'Enhanced evaluation with wider provider separation and three alternating degradation episodes '
      + '(200 s). Accuracy-heavy weights (50/25/25) amplify the quality difference, and a lower '
      + 'availability threshold makes the baseline even more brittle.',
    steps: [
      'Extreme provider profiles: A accuracy [90–99%] vs B accuracy [38–62%], non-overlapping latency',
      'Three alternating episodes (A→B→A); proposed method adapts proactively at each one',
      'After each recovery, QoS-aware switches back (gap 0.156 >> hysteresis 0.05) — baseline does not',
      'Advantage plot shows three distinct positive windows separated by near-zero inter-episode gaps',
    ],
    demonstrates: [
      'Sharp utility crossover in trade-off plot',
      'Repeated switch-back cycles demonstrating proactive adaptation',
      'Wider advantage gap due to accuracy-heavy weights',
    ],
    alphaSteps: 21,
    provA: { acc: [0.90, 0.99], lat: [55, 95], rel: [0.92, 0.99] },
    provB: { acc: [0.38, 0.62], lat: [2, 9],   rel: [0.52, 0.76] },
    simDuration: 200, recoveryDuration: 10, episodeCount: 3,
    degradeRate: [0.05, 0.18],
    onsetRange: [12, 20], degradeWindow: [8, 16], failWindow: [8, 14], interGap: [5, 12],
    nomA: { acc: 0.97, lat: 18, rel: 0.995 },
    nomB: { acc: 0.68, lat: 5,  rel: 0.82 },
    wAcc: 0.50, wLat: 0.25, wRel: 0.25,
    utilThreshold: 0.50, hysteresis: 0.05, availThreshold: 0.12,
  },
  stress01: {
    label: 'stress01', tagColor: '#b91c1c', tagBg: '#fee2e2',
    description:
      'Adversarial stress test: extreme provider contrast (A: 93–99% accuracy vs B: 28–55%), '
      + 'four rapid degradation episodes over 300 s, very aggressive degradation rates, '
      + 'and a very low availability baseline threshold (0.08) — the baseline almost never '
      + 'reacts in time. Designed to make the QoS-aware advantage unmistakably large.',
    steps: [
      'Maximum provider separation: A utility ≈ 0.96, B utility ≈ 0.68 at healthy state',
      'Four alternating episodes (A→B→A→B); fast rates (0.08–0.25 acc/s) create steep drops',
      'Long hard-fail windows (10–20 s) during which the baseline suffers near-zero utility',
      'Very low avail-threshold (0.08) means baseline switches even later than in other scenarios',
      'QoS-aware proactively switches back at every recovery (gap 0.286 >> hysteresis 0.04)',
    ],
    demonstrates: [
      'Maximum discriminability between QoS-aware and availability-based selection',
      'System robustness under four consecutive failure-recovery cycles',
      'Very sharp trade-off crossover (provider B dominates only below α ≈ 0.15)',
    ],
    alphaSteps: 21,
    provA: { acc: [0.93, 0.99], lat: [65, 100], rel: [0.93, 0.995] },
    provB: { acc: [0.28, 0.55], lat: [1.5, 7],  rel: [0.38, 0.65] },
    simDuration: 300, recoveryDuration: 8, episodeCount: 4,
    degradeRate: [0.08, 0.25],
    onsetRange: [10, 20], degradeWindow: [6, 12], failWindow: [10, 20], interGap: [5, 12],
    nomA: { acc: 0.98, lat: 14, rel: 0.998 },
    nomB: { acc: 0.55, lat: 4,  rel: 0.72 },
    wAcc: 0.55, wLat: 0.20, wRel: 0.25,
    utilThreshold: 0.45, hysteresis: 0.04, availThreshold: 0.08,
  },
}

// ─── Simulation parameters (user-editable, initialized from scenario preset) ──

interface SimParams {
  runs: number; seed: number
  provAAccMin: number; provAAccMax: number
  provALatMin: number; provALatMax: number
  provARelMin: number; provARelMax: number
  provBAccMin: number; provBAccMax: number
  provBLatMin: number; provBLatMax: number
  provBRelMin: number; provBRelMax: number
  simDuration: number; episodeCount: number; recoveryDuration: number
  degradeRateMin: number; degradeRateMax: number
  onsetMin: number; onsetMax: number
  degradeWinMin: number; degradeWinMax: number
  failWinMin: number; failWinMax: number
  interGapMin: number; interGapMax: number
  nomAAcc: number; nomALat: number; nomARel: number
  nomBAcc: number; nomBLat: number; nomBRel: number
  wAcc: number; wLat: number
  utilThreshold: number; hysteresis: number; availThreshold: number
}

function paramsFromScenario(s: ScenarioConfig, runs = 30, seed = 1234): SimParams {
  return {
    runs, seed,
    provAAccMin: s.provA.acc[0], provAAccMax: s.provA.acc[1],
    provALatMin: s.provA.lat[0], provALatMax: s.provA.lat[1],
    provARelMin: s.provA.rel[0], provARelMax: s.provA.rel[1],
    provBAccMin: s.provB.acc[0], provBAccMax: s.provB.acc[1],
    provBLatMin: s.provB.lat[0], provBLatMax: s.provB.lat[1],
    provBRelMin: s.provB.rel[0], provBRelMax: s.provB.rel[1],
    simDuration: s.simDuration, episodeCount: s.episodeCount, recoveryDuration: s.recoveryDuration,
    degradeRateMin: s.degradeRate[0], degradeRateMax: s.degradeRate[1],
    onsetMin: s.onsetRange[0], onsetMax: s.onsetRange[1],
    degradeWinMin: s.degradeWindow[0], degradeWinMax: s.degradeWindow[1],
    failWinMin: s.failWindow[0], failWinMax: s.failWindow[1],
    interGapMin: s.interGap[0], interGapMax: s.interGap[1],
    nomAAcc: s.nomA.acc, nomALat: s.nomA.lat, nomARel: s.nomA.rel,
    nomBAcc: s.nomB.acc, nomBLat: s.nomB.lat, nomBRel: s.nomB.rel,
    wAcc: s.wAcc, wLat: s.wLat,
    utilThreshold: s.utilThreshold, hysteresis: s.hysteresis, availThreshold: s.availThreshold,
  }
}

// ─── PRNG (mulberry32 — consistent results for same seed) ─────────────────────

function makePRNG(seed: number) {
  let s = (seed >>> 0) || 1
  const next = (): number => {
    s = (s + 0x6D2B79F5) >>> 0
    let t = Math.imul(s ^ (s >>> 15), 1 | s)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 0x100000000
  }
  return {
    next,
    uniform: (a: number, b: number) => a + next() * (b - a),
    choice:  <T,>(arr: T[]): T => arr[Math.floor(next() * arr.length)],
  }
}

// ─── Statistics helpers ────────────────────────────────────────────────────────

function percentile(sorted: number[], p: number): number {
  const idx = Math.floor(sorted.length * p)
  return sorted[Math.min(idx, sorted.length - 1)]
}
function stats(arr: number[]): [number, number, number] {
  const s = [...arr].sort((a, b) => a - b)
  const n = s.length
  const med = n % 2 === 0 ? (s[n/2-1] + s[n/2]) / 2 : s[Math.floor(n/2)]
  return [med, percentile(s, 0.10), percentile(s, 0.90)]
}

// ─── Simulation engine ────────────────────────────────────────────────────────

const MAX_LAT_MS = 100

function computeUtility(acc: number, lat: number, rel: number,
                        wA: number, wL: number, wR: number): number {
  return wA * acc + wL * (1 - Math.min(1, lat / MAX_LAT_MS)) + wR * rel
}

interface Episode { onset: number; rate: number; failAt: number; recoverAt: number }
interface QoS { acc: number; lat: number; rel: number }

function providerQosAt(t: number, nom: QoS, eps: Episode[], recDur: number): QoS {
  for (const ep of eps) {
    const recEnd = ep.recoverAt + recDur
    if (t >= ep.onset && t < ep.failAt) {
      const e = t - ep.onset
      return {
        acc: Math.max(0, nom.acc - e * ep.rate),
        lat: nom.lat * (1 + e * ep.rate * 2),
        rel: Math.max(0, nom.rel - e * ep.rate * 0.8),
      }
    }
    if (t >= ep.failAt && t < ep.recoverAt) {
      return { acc: 0, lat: nom.lat * 5, rel: 0 }
    }
    if (t >= ep.recoverAt && t < recEnd) {
      const f = (t - ep.recoverAt) / recDur
      return { acc: nom.acc * f, lat: nom.lat * (4 - 3 * f), rel: nom.rel * f }
    }
  }
  return { ...nom }
}

// ─── Chart data types ──────────────────────────────────────────────────────────

export interface TradeoffPoint {
  alpha: number
  medA: number; p10A: number; p90A: number; bwA: number  // bw = p90-p10 for stacked Area
  medB: number; p10B: number; p90B: number; bwB: number
}

export interface DegradPoint {
  t: number
  medQ: number; p10Q: number; p90Q: number; bwQ: number
  medAv: number; p10Av: number; p90Av: number; bwAv: number
  medAdv: number; p10Adv: number; p90Adv: number; bwAdv: number
}

export interface SimResults {
  tradeoff: TradeoffPoint[]
  degrad: DegradPoint[]
  params: SimParams
  scenarioKey: string
  elapsedMs: number
  crossoverAlpha: number | null  // alpha where A and B medians cross
  degradWindows: Array<{ onset: number; fail: number }>
}

// ─── Tradeoff experiment (browser) ────────────────────────────────────────────

function runTradeoffSim(p: SimParams): TradeoffPoint[] {
  const alphas = Array.from({ length: p.runs > 0 ? 21 : 0 }, (_, i) => i / 20)
  const byAlpha = new Map<number, { uA: number[]; uB: number[] }>()
  for (const a of alphas) byAlpha.set(a, { uA: [], uB: [] })

  const wRel = Math.max(0, 1 - p.wAcc - p.wLat)

  for (let run = 0; run < p.runs; run++) {
    const rng = makePRNG(p.seed + run)
    const provA = {
      acc: rng.uniform(p.provAAccMin, p.provAAccMax),
      lat: rng.uniform(p.provALatMin, p.provALatMax),
      rel: rng.uniform(p.provARelMin, p.provARelMax),
    }
    const provB = {
      acc: rng.uniform(p.provBAccMin, p.provBAccMax),
      lat: rng.uniform(p.provBLatMin, p.provBLatMax),
      rel: rng.uniform(p.provBRelMin, p.provBRelMax),
    }
    for (const alpha of alphas) {
      const wA = alpha, wL = (1 - alpha) / 2, wR = (1 - alpha) / 2
      const uA = computeUtility(provA.acc, provA.lat, provA.rel, wA, wL, wR)
      const uB = computeUtility(provB.acc, provB.lat, provB.rel, wA, wL, wR)
      const d = byAlpha.get(alpha)!
      d.uA.push(uA); d.uB.push(uB)
    }
  }

  const result: TradeoffPoint[] = alphas.map(alpha => {
    const d = byAlpha.get(alpha)!
    const [medA, p10A, p90A] = stats(d.uA)
    const [medB, p10B, p90B] = stats(d.uB)
    return { alpha, medA, p10A, p90A, bwA: p90A - p10A, medB, p10B, p90B, bwB: p90B - p10B }
  })

  void wRel // suppress unused
  return result
}

// ─── Degradation experiment (browser) ────────────────────────────────────────

function runDegradSim(p: SimParams): { points: DegradPoint[]; windows: Array<{ onset: number; fail: number }> } {
  const utilByT = new Map<number, { q: number[]; av: number[] }>()
  for (let t = 0; t <= p.simDuration; t++) utilByT.set(t, { q: [], av: [] })

  const wRel = Math.max(0, 1 - p.wAcc - p.wLat)
  const nomA: QoS = { acc: p.nomAAcc, lat: p.nomALat, rel: p.nomARel }
  const nomB: QoS = { acc: p.nomBAcc, lat: p.nomBLat, rel: p.nomBRel }

  // Collect degradation windows across runs for shading
  const allOnsets: number[][] = Array.from({ length: p.episodeCount }, () => [])
  const allFails:  number[][] = Array.from({ length: p.episodeCount }, () => [])

  for (let run = 0; run < p.runs; run++) {
    const rng = makePRNG(p.seed + run)

    // Build episodes
    const epA: Episode[] = [], epB: Episode[] = []
    let firstDeg: 'a' | 'b' = rng.choice(['a', 'b'] as const)
    let curStart = 0

    for (let i = 0; i < p.episodeCount; i++) {
      const deg: 'a' | 'b' = i % 2 === 0 ? firstDeg : (firstDeg === 'a' ? 'b' : 'a')
      const onset = i === 0
        ? rng.uniform(p.onsetMin, p.onsetMax)
        : curStart + rng.uniform(p.interGapMin, p.interGapMax)
      const rate    = rng.uniform(p.degradeRateMin, p.degradeRateMax)
      const failAt  = onset + rng.uniform(p.degradeWinMin, p.degradeWinMax)
      const recAt   = failAt + rng.uniform(p.failWinMin, p.failWinMax)
      const recEnd  = recAt + p.recoveryDuration
      curStart = recEnd

      const ep: Episode = { onset, rate, failAt, recoverAt: recAt }
      if (deg === 'a') epA.push(ep); else epB.push(ep)

      allOnsets[i].push(onset)
      allFails[i].push(failAt)
    }

    // Simulate both methods
    for (const method of ['qos_aware', 'avail_based'] as const) {
      let active: 'a' | 'b' = 'a'

      for (let t = 0; t <= p.simDuration; t++) {
        const qA = providerQosAt(t, nomA, epA, p.recoveryDuration)
        const qB = providerQosAt(t, nomB, epB, p.recoveryDuration)
        const qMap = { a: qA, b: qB }

        if (method === 'qos_aware') {
          const aq = qMap[active]
          const uActive = computeUtility(aq.acc, aq.lat, aq.rel, p.wAcc, p.wLat, wRel)
          const alt: 'a' | 'b' = active === 'a' ? 'b' : 'a'
          const aq2 = qMap[alt]
          const uAlt = computeUtility(aq2.acc, aq2.lat, aq2.rel, p.wAcc, p.wLat, wRel)
          if ((uActive < p.utilThreshold || uAlt > uActive + p.hysteresis) && uAlt > uActive) {
            active = alt
          }
        } else {
          if (qMap[active].rel < p.availThreshold) {
            const alt: 'a' | 'b' = active === 'a' ? 'b' : 'a'
            if (qMap[alt].rel >= p.availThreshold) active = alt
          }
        }

        const qa = qMap[active]
        const util = computeUtility(qa.acc, qa.lat, qa.rel, p.wAcc, p.wLat, wRel)
        const d = utilByT.get(t)!
        if (method === 'qos_aware') d.q.push(util); else d.av.push(util)
      }
    }
  }

  // Compute degradation windows (median across runs)
  const windows = allOnsets.map((onsets, i) => ({
    onset: onsets.reduce((a, b) => a + b, 0) / onsets.length,
    fail:  allFails[i].reduce((a, b) => a + b, 0) / allFails[i].length,
  }))

  // Build chart data
  const points: DegradPoint[] = []
  for (let t = 0; t <= p.simDuration; t++) {
    const d = utilByT.get(t)!
    const [medQ, p10Q, p90Q] = stats(d.q)
    const [medAv, p10Av, p90Av] = stats(d.av)
    const adv = d.q.map((q, i) => q - (d.av[i] ?? 0))
    const [medAdv, p10Adv, p90Adv] = stats(adv)
    points.push({
      t,
      medQ, p10Q, p90Q, bwQ: p90Q - p10Q,
      medAv, p10Av, p90Av, bwAv: p90Av - p10Av,
      medAdv, p10Adv, p90Adv, bwAdv: p90Adv - p10Adv,
    })
  }
  return { points, windows }
}

// ─── Custom tooltip ────────────────────────────────────────────────────────────

const FmtTooltip = ({ active, payload, label, xLabel = '' }: any) => {
  if (!active || !payload?.length) return null
  return (
    <div style={{
      background: 'var(--bg-surface)', border: '1px solid var(--border)',
      borderRadius: 6, padding: '8px 12px', fontSize: '0.75rem',
      boxShadow: 'var(--shadow)',
    }}>
      <div style={{ fontWeight: 700, marginBottom: 4, color: 'var(--text-primary)' }}>
        {xLabel} {typeof label === 'number' ? label.toFixed(2) : label}
      </div>
      {payload.map((p: any) => (
        p.dataKey && !p.dataKey.startsWith('bw') && !p.dataKey.startsWith('p10') && !p.dataKey.startsWith('p90') ? (
          <div key={p.dataKey} style={{ color: p.stroke || p.fill, marginBottom: 2 }}>
            {p.name}: <strong>{typeof p.value === 'number' ? p.value.toFixed(3) : p.value}</strong>
          </div>
        ) : null
      ))}
    </div>
  )
}

// ─── Chart colour constants ────────────────────────────────────────────────────

const C_A = '#1d4ed8'       // provider A / proposed
const C_B = '#b91c1c'       // provider B / baseline
const C_ADV = '#0891b2'     // advantage (teal)
const BAND_ALPHA = 0.13

// ─── Main component ───────────────────────────────────────────────────────────

interface Props { simSpeed: number }

const SimulationView: React.FC<Props> = ({ simSpeed }) => {
  const [scenarioKey, setScenarioKey] = useState<string>('improved01')
  const [params, setParams] = useState<SimParams>(() => paramsFromScenario(SCENARIOS.improved01))
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [running, setRunning] = useState(false)
  const [results, setResults] = useState<SimResults | null>(null)
  const [playT, setPlayT] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [localSpeed, setLocalSpeed] = useState(2)
  const intervalRef = useRef<number | null>(null)

  const effectiveSpeed = simSpeed > 1 ? simSpeed : localSpeed

  // Animation loop for degradation playback
  useEffect(() => {
    if (!playing || !results) return
    if (playT >= results.params.simDuration) { setPlaying(false); return }
    intervalRef.current = window.setInterval(() => {
      setPlayT(t => {
        const next = t + effectiveSpeed
        if (next >= results.params.simDuration) { setPlaying(false); return results.params.simDuration }
        return next
      })
    }, 50)
    return () => { if (intervalRef.current) clearInterval(intervalRef.current) }
  }, [playing, effectiveSpeed, results, playT])

  const handleSelectScenario = useCallback((key: string) => {
    setScenarioKey(key)
    setParams(p => paramsFromScenario(SCENARIOS[key], p.runs, p.seed))
    setResults(null)
    setPlayT(0)
    setPlaying(false)
  }, [])

  const setParam = useCallback(<K extends keyof SimParams,>(key: K, val: SimParams[K]) => {
    setParams(p => ({ ...p, [key]: val }))
  }, [])

  const handleRun = useCallback(() => {
    setRunning(true)
    setResults(null)
    setPlayT(0)
    setPlaying(false)
    setTimeout(() => {
      const t0 = performance.now()
      const tradeoff = runTradeoffSim(params)
      const { points: degrad, windows } = runDegradSim(params)

      // Find crossover alpha
      let crossover: number | null = null
      for (let i = 0; i < tradeoff.length - 1; i++) {
        if ((tradeoff[i].medA < tradeoff[i].medB) !== (tradeoff[i+1].medA < tradeoff[i+1].medB)) {
          crossover = tradeoff[i].alpha
          break
        }
      }
      setResults({
        tradeoff, degrad, params, scenarioKey,
        elapsedMs: Math.round(performance.now() - t0),
        crossoverAlpha: crossover,
        degradWindows: windows,
      })
      setRunning(false)
    }, 10)
  }, [params, scenarioKey])

  const handlePlayPause = () => {
    if (playT >= (results?.params.simDuration ?? 0)) { setPlayT(0); setPlaying(true); return }
    setPlaying(p => !p)
  }
  const handleRewind = () => { setPlayT(0); setPlaying(false) }

  const wRel = Math.max(0, +(1 - params.wAcc - params.wLat).toFixed(3))
  const degradData = results
    ? results.degrad.filter(d => d.t <= Math.round(playT))
    : []
  const simDur = results?.params.simDuration ?? params.simDuration

  // ─── Render ────────────────────────────────────────────────────────────────
  return (
    <div className="page-container">
      {/* ── Scenario Selection ──────────────────────────────────────────────── */}
      <div className="section">
        <div className="section-title">Simulation Scenario</div>
        <div className="sim-scenario-cards">
          {Object.entries(SCENARIOS).map(([key, sc]) => (
            <div
              key={key}
              className={`sim-scenario-card${scenarioKey === key ? ' selected' : ''}`}
              onClick={() => handleSelectScenario(key)}
            >
              <div className="sim-scenario-card-name">{key}</div>
              <span
                className="sim-scenario-card-badge"
                style={{ color: sc.tagColor, background: sc.tagBg }}
              >
                {sc.label}
              </span>
              <div className="sim-scenario-card-desc">{sc.description}</div>
              <div className="sim-scenario-steps">
                {sc.steps.map((step, i) => (
                  <div key={i} className="sim-scenario-step">
                    <span className="sim-scenario-step-num">{i + 1}</span>
                    <span>{step}</span>
                  </div>
                ))}
              </div>
              {sc.demonstrates.length > 0 && (
                <div style={{ marginTop: 10, padding: '8px 10px', background: '#dbeafe', border: '1px solid #93c5fd', borderRadius: 6, fontSize: '0.72rem', color: '#1d4ed8' }}>
                  <strong>Demonstrates:</strong>&nbsp;{sc.demonstrates.join(' · ')}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* ── Simulation Settings ─────────────────────────────────────────────── */}
      <div className="section">
        <div className="section-title">
          Simulation Settings
          <button
            className="btn btn-ghost btn-sm"
            style={{ marginLeft: 8 }}
            onClick={() => setShowAdvanced(v => !v)}
          >
            {showAdvanced ? '▲ Hide advanced' : '▼ Advanced parameters'}
          </button>
        </div>

        {/* Basic settings always visible */}
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginBottom: 16 }}>
          <div className="sim-param-row" style={{ minWidth: 140 }}>
            <label className="sim-param-label">Runs</label>
            <input className="input" type="number" min={1} max={200} step={1}
              value={params.runs}
              onChange={e => setParam('runs', +e.target.value)} />
          </div>
          <div className="sim-param-row" style={{ minWidth: 140 }}>
            <label className="sim-param-label">Base seed</label>
            <input className="input" type="number" min={0} step={1}
              value={params.seed}
              onChange={e => setParam('seed', +e.target.value)} />
          </div>
          <div className="sim-param-row" style={{ minWidth: 140 }}>
            <label className="sim-param-label">Episodes</label>
            <input className="input" type="number" min={1} max={4} step={1}
              value={params.episodeCount}
              onChange={e => setParam('episodeCount', +e.target.value)} />
          </div>
          <div className="sim-param-row" style={{ minWidth: 140 }}>
            <label className="sim-param-label">Sim duration (s)</label>
            <input className="input" type="number" min={30} max={600} step={10}
              value={params.simDuration}
              onChange={e => setParam('simDuration', +e.target.value)} />
          </div>
        </div>

        {/* Advanced parameters (collapsible) */}
        {showAdvanced && (
          <div className="sim-params-grid">
            {/* Provider A */}
            <div className="card card-sm sim-param-group">
              <div className="card-title" style={{ marginBottom: 12 }}>
                Provider A — Quality sensor
                <span style={{ marginLeft: 8, fontSize: '0.7rem', color: C_A, fontWeight: 400 }}>accurate · slow · reliable</span>
              </div>
              {(['acc', 'lat', 'rel'] as const).map(dim => {
                const labels = { acc: 'Accuracy', lat: 'Latency (ms)', rel: 'Reliability' }
                const minK = `provA${dim.charAt(0).toUpperCase()+dim.slice(1)}Min` as keyof SimParams
                const maxK = `provA${dim.charAt(0).toUpperCase()+dim.slice(1)}Max` as keyof SimParams
                return (
                  <div key={dim} className="sim-param-row">
                    <label className="sim-param-label">{labels[dim]} range</label>
                    <div className="sim-param-range">
                      <input className="input" type="number" step={dim==='lat'?1:0.01}
                        value={params[minK] as number} onChange={e => setParam(minK, +e.target.value)} />
                      <span className="sim-param-range-sep">–</span>
                      <input className="input" type="number" step={dim==='lat'?1:0.01}
                        value={params[maxK] as number} onChange={e => setParam(maxK, +e.target.value)} />
                    </div>
                  </div>
                )
              })}
            </div>

            {/* Provider B */}
            <div className="card card-sm sim-param-group">
              <div className="card-title" style={{ marginBottom: 12 }}>
                Provider B — Fast sensor
                <span style={{ marginLeft: 8, fontSize: '0.7rem', color: C_B, fontWeight: 400 }}>fast · noisy · less reliable</span>
              </div>
              {(['acc', 'lat', 'rel'] as const).map(dim => {
                const labels = { acc: 'Accuracy', lat: 'Latency (ms)', rel: 'Reliability' }
                const minK = `provB${dim.charAt(0).toUpperCase()+dim.slice(1)}Min` as keyof SimParams
                const maxK = `provB${dim.charAt(0).toUpperCase()+dim.slice(1)}Max` as keyof SimParams
                return (
                  <div key={dim} className="sim-param-row">
                    <label className="sim-param-label">{labels[dim]} range</label>
                    <div className="sim-param-range">
                      <input className="input" type="number" step={dim==='lat'?1:0.01}
                        value={params[minK] as number} onChange={e => setParam(minK, +e.target.value)} />
                      <span className="sim-param-range-sep">–</span>
                      <input className="input" type="number" step={dim==='lat'?1:0.01}
                        value={params[maxK] as number} onChange={e => setParam(maxK, +e.target.value)} />
                    </div>
                  </div>
                )
              })}
            </div>

            {/* Algorithm Parameters */}
            <div className="card card-sm sim-param-group">
              <div className="card-title" style={{ marginBottom: 12 }}>Algorithm Parameters</div>
              <div className="sim-param-row">
                <label className="sim-param-label">Utility weight — accuracy (wAcc)</label>
                <input className="input" type="number" min={0} max={1} step={0.05}
                  value={params.wAcc} onChange={e => setParam('wAcc', +e.target.value)} />
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">Utility weight — latency (wLat)</label>
                <input className="input" type="number" min={0} max={1} step={0.05}
                  value={params.wLat} onChange={e => setParam('wLat', +e.target.value)} />
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">Reliability weight (derived)</label>
                <div className="input" style={{ background: 'var(--bg-elevated)', color: 'var(--text-secondary)' }}>
                  wRel = {wRel.toFixed(2)}
                </div>
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">QoS-aware failover threshold</label>
                <input className="input" type="number" min={0} max={1} step={0.05}
                  value={params.utilThreshold} onChange={e => setParam('utilThreshold', +e.target.value)} />
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">QoS switch-back hysteresis</label>
                <input className="input" type="number" min={0} max={0.5} step={0.01}
                  value={params.hysteresis} onChange={e => setParam('hysteresis', +e.target.value)} />
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">Availability-based threshold</label>
                <input className="input" type="number" min={0} max={1} step={0.01}
                  value={params.availThreshold} onChange={e => setParam('availThreshold', +e.target.value)} />
              </div>
              <div className="sim-param-row">
                <label className="sim-param-label">Degradation rate range (acc/s)</label>
                <div className="sim-param-range">
                  <input className="input" type="number" min={0} max={1} step={0.01}
                    value={params.degradeRateMin} onChange={e => setParam('degradeRateMin', +e.target.value)} />
                  <span className="sim-param-range-sep">–</span>
                  <input className="input" type="number" min={0} max={1} step={0.01}
                    value={params.degradeRateMax} onChange={e => setParam('degradeRateMax', +e.target.value)} />
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Run bar */}
        <div className="sim-run-bar">
          <button
            className="btn btn-primary btn-lg"
            onClick={handleRun}
            disabled={running || params.runs < 1}
          >
            {running ? <><span className="loading-spinner" />&nbsp;Computing…</> : '▶  Run Simulation'}
          </button>
          {results && (
            <span style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
              {results.params.runs} runs · seed {results.params.seed} · computed in {results.elapsedMs} ms
            </span>
          )}
          {results && (
            <button
              className="btn btn-outline btn-sm"
              onClick={() => setParams(paramsFromScenario(SCENARIOS[scenarioKey], params.runs, params.seed))}
            >
              Reset to preset
            </button>
          )}
        </div>
      </div>

      {/* ── Results ─────────────────────────────────────────────────────────── */}
      {results && (
        <>
          {/* Charts grid: tradeoff left, degradation right */}
          <div className="sim-charts-grid">
            {/* ── Trade-off chart ── */}
            <div className="sim-chart-card">
              <div className="sim-chart-title">
                QoS Trade-off: Provider Utility vs. Accuracy Weight
                {results.crossoverAlpha !== null && (
                  <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', fontWeight: 400 }}>
                    crossover α ≈ {results.crossoverAlpha.toFixed(2)}
                  </span>
                )}
              </div>
              <ResponsiveContainer width="100%" height={280}>
                <ComposedChart data={results.tradeoff} margin={{ top: 4, right: 16, bottom: 4, left: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis dataKey="alpha" tickCount={6}
                    label={{ value: 'Accuracy weight α', position: 'insideBottom', offset: -4, fontSize: 11 }}
                    domain={[0, 1]} tickFormatter={v => v.toFixed(1)} tick={{ fontSize: 10 }} />
                  <YAxis domain={[0, 1.05]} tickFormatter={v => v.toFixed(1)} tick={{ fontSize: 10 }} />
                  <Tooltip content={<FmtTooltip xLabel="α =" />} />
                  {results.crossoverAlpha !== null && (
                    <ReferenceLine x={results.crossoverAlpha} stroke="#6b7280" strokeDasharray="4 2"
                      label={{ value: `α≈${results.crossoverAlpha.toFixed(2)}`, fontSize: 9, fill: '#6b7280', position: 'insideTopRight' }} />
                  )}
                  {/* Provider A band */}
                  <Area type="monotone" dataKey="p10A" stackId="bandA" stroke="none" fill="transparent" isAnimationActive={false} legendType="none" />
                  <Area type="monotone" dataKey="bwA" stackId="bandA" stroke="none" fill={C_A} fillOpacity={BAND_ALPHA} isAnimationActive={false} legendType="none" />
                  {/* Provider B band */}
                  <Area type="monotone" dataKey="p10B" stackId="bandB" stroke="none" fill="transparent" isAnimationActive={false} legendType="none" />
                  <Area type="monotone" dataKey="bwB" stackId="bandB" stroke="none" fill={C_B} fillOpacity={BAND_ALPHA} isAnimationActive={false} legendType="none" />
                  {/* Median lines */}
                  <Line type="monotone" dataKey="medA" stroke={C_A} strokeWidth={2.2} dot={false}
                    name="Provider A (median)" isAnimationActive={false} />
                  <Line type="monotone" dataKey="medB" stroke={C_B} strokeWidth={2.2} strokeDasharray="6 3" dot={false}
                    name="Provider B (median)" isAnimationActive={false} />
                </ComposedChart>
              </ResponsiveContainer>
              <div className="chart-legend-row">
                <div className="chart-legend-item">
                  <div className="chart-legend-swatch" style={{ background: C_A }} />
                  Provider A — quality sensor (median)
                </div>
                <div className="chart-legend-item">
                  <div className="chart-legend-swatch" style={{ background: C_B, opacity: 0.5 }} />
                  Provider B — fast sensor (median, dashed)
                </div>
                <div className="chart-legend-item" style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>
                  Shaded = p10–p90
                </div>
              </div>
            </div>

            {/* ── Degradation utility chart ── */}
            <div className="sim-chart-card">
              <div className="sim-chart-title">
                Degradation: Utility over Time
                <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', fontWeight: 400 }}>
                  {results.params.runs} runs
                </span>
              </div>

              {/* Playback controls */}
              <div className="playback-controls">
                <button className="btn btn-outline btn-sm" onClick={handleRewind} title="Rewind">⏮</button>
                <button className="btn btn-primary btn-sm" onClick={handlePlayPause}>
                  {playing ? '⏸' : (playT >= simDur ? '↺' : '▶')}
                </button>
                <div className="playback-progress" onClick={e => {
                  const rect = e.currentTarget.getBoundingClientRect()
                  setPlayT(Math.round(((e.clientX - rect.left) / rect.width) * simDur))
                  setPlaying(false)
                }}>
                  <div className="playback-progress-fill" style={{ width: `${(playT / simDur) * 100}%` }} />
                </div>
                <span className="playback-time">{Math.round(playT)}s / {simDur}s</span>
                <div className="speed-btns">
                  {[1, 2, 5, 10].map(spd => (
                    <button key={spd} className={`speed-btn${localSpeed === spd ? ' active' : ''}`}
                      onClick={() => setLocalSpeed(spd)}>
                      {spd}×
                    </button>
                  ))}
                </div>
              </div>

              <ResponsiveContainer width="100%" height={220}>
                <ComposedChart data={degradData} margin={{ top: 4, right: 16, bottom: 4, left: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis dataKey="t" domain={[0, simDur]} type="number" tickFormatter={v => `${v}s`} tick={{ fontSize: 10 }} />
                  <YAxis domain={[-0.02, 1.08]} tickFormatter={v => v.toFixed(1)} tick={{ fontSize: 10 }} />
                  <Tooltip content={<FmtTooltip xLabel="t =" />} />
                  {/* Degradation windows */}
                  {results.degradWindows.map((w, i) => (
                    <ReferenceLine key={i} x={w.onset} stroke="#9ca3af" strokeDasharray="3 3" />
                  ))}
                  {/* Bands */}
                  <Area type="monotone" dataKey="p10Q" stackId="bq" stroke="none" fill="transparent" isAnimationActive={false} legendType="none" />
                  <Area type="monotone" dataKey="bwQ" stackId="bq" stroke="none" fill={C_A} fillOpacity={BAND_ALPHA} isAnimationActive={false} legendType="none" />
                  <Area type="monotone" dataKey="p10Av" stackId="bav" stroke="none" fill="transparent" isAnimationActive={false} legendType="none" />
                  <Area type="monotone" dataKey="bwAv" stackId="bav" stroke="none" fill={C_B} fillOpacity={BAND_ALPHA} isAnimationActive={false} legendType="none" />
                  <Line type="monotone" dataKey="medQ" stroke={C_A} strokeWidth={2.2} dot={false}
                    name="QoS-aware (median)" isAnimationActive={false} />
                  <Line type="monotone" dataKey="medAv" stroke={C_B} strokeWidth={2.2} strokeDasharray="6 3" dot={false}
                    name="Availability-based (median)" isAnimationActive={false} />
                </ComposedChart>
              </ResponsiveContainer>
              <div className="chart-legend-row">
                <div className="chart-legend-item">
                  <div className="chart-legend-swatch" style={{ background: C_A }} />
                  Proposed — QoS-aware (median)
                </div>
                <div className="chart-legend-item">
                  <div className="chart-legend-swatch" style={{ background: C_B, opacity: 0.5 }} />
                  Baseline — availability-based (median, dashed)
                </div>
              </div>
            </div>
          </div>

          {/* ── Advantage chart (full width) ── */}
          <div className="sim-chart-card" style={{ marginTop: 16 }}>
            <div className="sim-chart-title">
              Utility Advantage: Proposed − Baseline (animated replay above)
            </div>
            <ResponsiveContainer width="100%" height={180}>
              <ComposedChart data={degradData} margin={{ top: 4, right: 16, bottom: 8, left: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="t" domain={[0, simDur]} type="number"
                  label={{ value: 'Simulation time (s)', position: 'insideBottom', offset: -4, fontSize: 11 }}
                  tickFormatter={v => `${v}s`} tick={{ fontSize: 10 }} />
                <YAxis tickFormatter={v => v.toFixed(2)} tick={{ fontSize: 10 }} />
                <Tooltip content={<FmtTooltip xLabel="t =" />} />
                {results.degradWindows.map((w, i) => (
                  <ReferenceLine key={i} x={w.onset} stroke="#9ca3af" strokeDasharray="3 3" />
                ))}
                <ReferenceLine y={0} stroke="#374151" strokeDasharray="4 2"
                  label={{ value: 'No advantage', fontSize: 9, fill: '#6b7280', position: 'insideRight' }} />
                {/* Advantage band */}
                <Area type="monotone" dataKey="p10Adv" stackId="badv" stroke="none" fill="transparent" isAnimationActive={false} legendType="none" />
                <Area type="monotone" dataKey="bwAdv" stackId="badv" stroke="none" fill={C_ADV} fillOpacity={0.18} isAnimationActive={false} legendType="none" />
                <Line type="monotone" dataKey="medAdv" stroke={C_ADV} strokeWidth={2.2} dot={false}
                  name="Advantage (median)" isAnimationActive={false} />
                <Line type="monotone" dataKey="p10Adv" stroke={C_ADV} strokeWidth={0.8} strokeDasharray="3 2" dot={false}
                  name="p10 / p90" isAnimationActive={false} />
                <Line type="monotone" dataKey="p90Adv" stroke={C_ADV} strokeWidth={0.8} strokeDasharray="3 2" dot={false}
                  isAnimationActive={false} legendType="none" />
              </ComposedChart>
            </ResponsiveContainer>
            <div className="chart-legend-row">
              <div className="chart-legend-item">
                <div className="chart-legend-swatch" style={{ background: C_ADV }} />
                Utility advantage (QoS-aware − baseline), median
              </div>
              <div className="chart-legend-item">
                <div className="chart-legend-swatch dashed" style={{ color: C_ADV }} />
                p10 / p90
              </div>
              <div className="chart-legend-item" style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>
                Gray dashed verticals = median degradation onsets
              </div>
            </div>
          </div>
        </>
      )}

      {!results && !running && (
        <div className="empty-state">
          <div style={{ fontSize: '2rem' }}>📊</div>
          <div style={{ fontWeight: 600, color: 'var(--text-secondary)' }}>No results yet</div>
          <div>Configure parameters above and click <strong>Run Simulation</strong></div>
        </div>
      )}
    </div>
  )
}

export default SimulationView

import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { AnalysisSummary } from '../components/AnalysisSummary'
import type { TelemetryStatsResponse } from '../types'

const stats: TelemetryStatsResponse = {
  robots: {
    'robot-1': { lastSeq: 1, rateHz: 9.5, latencyMs: { mean: 15.2, p50: 14.0, p95: 22.1, p99: 28.0, max: 35.0 }, msgCount: 100, lastReceivedAt: '' },
    'robot-2': { lastSeq: 2, rateHz: 8.1, latencyMs: { mean: 45.0, p50: 44.0, p95: 60.0, p99: 70.0, max: 80.0 }, msgCount: 80, lastReceivedAt: '' },
  },
  aggregate: { robotCount: 2, totalMsgCount: 180, totalRateHz: 17.6, totalKbps: 5.0, latencyMs: { mean: 30.1, p50: 29.0, p95: 55.0, p99: 65.0, max: 80.0 } },
}

describe('AnalysisSummary', () => {
  it('renders section with testid', () => {
    render(<AnalysisSummary stats={stats} />)
    expect(screen.getByTestId('analysis-summary')).toBeTruthy()
  })

  it('renders heading', () => {
    render(<AnalysisSummary stats={stats} />)
    expect(screen.getByText('Analysis Summary')).toBeTruthy()
  })

  it('shows robot count', () => {
    render(<AnalysisSummary stats={stats} />)
    expect(screen.getByText('2')).toBeTruthy()
  })

  it('shows total message count', () => {
    render(<AnalysisSummary stats={stats} />)
    expect(screen.getByText('180')).toBeTruthy()
  })

  it('shows mean latency', () => {
    render(<AnalysisSummary stats={stats} />)
    expect(screen.getByText('30.1 ms')).toBeTruthy()
  })

  it('shows waiting message when stats is null', () => {
    render(<AnalysisSummary stats={null} />)
    expect(screen.getByText(/Waiting for telemetry/)).toBeTruthy()
  })
})

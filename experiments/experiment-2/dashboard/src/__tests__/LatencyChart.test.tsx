import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { LatencyChart } from '../components/LatencyChart'
import type { TelemetryStatsResponse } from '../types'

const stats: TelemetryStatsResponse = {
  robots: {
    'robot-1': { lastSeq: 1, rateHz: 9.5, latencyMs: { mean: 15.2, p50: 14.0, p95: 22.1, p99: 28.0, max: 35.0 }, msgCount: 100, lastReceivedAt: '' },
    'robot-2': { lastSeq: 2, rateHz: 8.1, latencyMs: { mean: 45.0, p50: 44.0, p95: 60.0, p99: 70.0, max: 80.0 }, msgCount: 80, lastReceivedAt: '' },
  },
  aggregate: { robotCount: 2, totalMsgCount: 180, totalRateHz: 17.6, totalKbps: 5.0, latencyMs: { mean: 30.1, p50: 29.0, p95: 55.0, p99: 65.0, max: 80.0 } },
}

describe('LatencyChart', () => {
  it('renders section with testid', () => {
    render(<LatencyChart stats={stats} />)
    expect(screen.getByTestId('latency-chart')).toBeTruthy()
  })

  it('renders heading', () => {
    render(<LatencyChart stats={stats} />)
    expect(screen.getByText(/Latency per Robot/)).toBeTruthy()
  })

  it('renders an svg element', () => {
    const { container } = render(<LatencyChart stats={stats} />)
    expect(container.querySelector('svg')).toBeTruthy()
  })

  it('renders a bar group for each robot', () => {
    render(<LatencyChart stats={stats} />)
    expect(screen.getByTestId('bar-robot-1')).toBeTruthy()
    expect(screen.getByTestId('bar-robot-2')).toBeTruthy()
  })

  it('shows no-data message when stats is null', () => {
    render(<LatencyChart stats={null} />)
    expect(screen.getByText(/No data yet/)).toBeTruthy()
  })

  it('shows no-data message when robots is empty', () => {
    render(<LatencyChart stats={{ robots: {}, aggregate: { robotCount: 0, totalMsgCount: 0, totalRateHz: 0, totalKbps: 0, latencyMs: { mean: 0, p50: 0, p95: 0, p99: 0, max: 0 } } }} />)
    expect(screen.getByText(/No data yet/)).toBeTruthy()
  })
})

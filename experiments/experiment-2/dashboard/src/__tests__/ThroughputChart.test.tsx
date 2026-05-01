import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { ThroughputChart } from '../components/ThroughputChart'
import type { TelemetryStatsResponse } from '../types'

const stats: TelemetryStatsResponse = {
  robots: {
    'robot-1': { lastSeq: 1, rateHz: 9.5, latencyMs: { mean: 15.2, p50: 14.0, p95: 22.1, p99: 28.0, max: 35.0 }, msgCount: 100, lastReceivedAt: '' },
    'robot-3': { lastSeq: 3, rateHz: 3.2, latencyMs: { mean: 80.0, p50: 78.0, p95: 110.0, p99: 130.0, max: 150.0 }, msgCount: 40, lastReceivedAt: '' },
  },
  aggregate: { robotCount: 2, totalMsgCount: 140, totalRateHz: 12.7, totalKbps: 3.0, latencyMs: { mean: 47.6, p50: 46.0, p95: 66.0, p99: 75.0, max: 150.0 } },
}

describe('ThroughputChart', () => {
  it('renders section with testid', () => {
    render(<ThroughputChart stats={stats} />)
    expect(screen.getByTestId('throughput-chart')).toBeTruthy()
  })

  it('renders heading', () => {
    render(<ThroughputChart stats={stats} />)
    expect(screen.getByText(/Throughput per Robot/)).toBeTruthy()
  })

  it('renders svg element', () => {
    const { container } = render(<ThroughputChart stats={stats} />)
    expect(container.querySelector('svg')).toBeTruthy()
  })

  it('renders a bar for each robot', () => {
    render(<ThroughputChart stats={stats} />)
    expect(screen.getByTestId('throughput-bar-robot-1')).toBeTruthy()
    expect(screen.getByTestId('throughput-bar-robot-3')).toBeTruthy()
  })

  it('shows no-data message when stats is null', () => {
    render(<ThroughputChart stats={null} />)
    expect(screen.getByText(/No data yet/)).toBeTruthy()
  })
})

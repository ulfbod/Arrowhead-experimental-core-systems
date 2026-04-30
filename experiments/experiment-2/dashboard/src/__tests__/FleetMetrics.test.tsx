import { render, screen, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { FleetMetrics } from '../components/FleetMetrics'
import { ConfigProvider } from '../config/context'
import type { TelemetryStatsResponse } from '../types'

const statsResponse: TelemetryStatsResponse = {
  robots: {
    'robot-1': { rateHz: 9.5, latency: { mean: 15.2, p50: 14.0, p95: 22.1, p99: 28.0, max: 35.0 }, msgCount: 100 },
    'robot-2': { rateHz: 8.1, latency: { mean: 45.0, p50: 44.0, p95: 60.0, p99: 70.0, max: 80.0 }, msgCount: 80 },
  },
  aggregate: { robotCount: 2, totalMsgCount: 180, meanLatencyMs: 30.1, p95LatencyMs: 55.0 },
}

function wrap() {
  return render(
    <ConfigProvider>
      <FleetMetrics />
    </ConfigProvider>,
  )
}

describe('FleetMetrics', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve(statsResponse),
    })
  })

  it('renders section', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('fleet-metrics')).toBeTruthy()
  })

  it('shows heading', async () => {
    await act(async () => { wrap() })
    expect(screen.getByText('Fleet Metrics')).toBeTruthy()
  })

  it('renders rows for each robot', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('robot-row-robot-1')).toBeTruthy()
    expect(screen.getByTestId('robot-row-robot-2')).toBeTruthy()
  })

  it('shows aggregate stats', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('aggregate-stats')).toBeTruthy()
  })

  it('shows no-data message when fetch returns empty', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({ robots: {}, aggregate: { robotCount: 0, totalMsgCount: 0, meanLatencyMs: 0, p95LatencyMs: 0 } }),
    })
    await act(async () => { wrap() })
    expect(screen.getByText(/No telemetry data yet/)).toBeTruthy()
  })
})

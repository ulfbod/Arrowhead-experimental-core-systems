import { render, screen, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { FleetMetrics } from '../components/FleetMetrics'
import { ConfigProvider } from '../config/context'
import type { TelemetryStatsResponse } from '../types'

const statsResponse: TelemetryStatsResponse = {
  robots: {
    'robot-1': { lastSeq: 1, rateHz: 9.5, latencyMs: { mean: 15.2, p50: 14.0, p95: 22.1, p99: 28.0, max: 35.0 }, msgCount: 100, lastReceivedAt: '' },
    'robot-2': { lastSeq: 2, rateHz: 8.1, latencyMs: { mean: 45.0, p50: 44.0, p95: 60.0, p99: 70.0, max: 80.0 }, msgCount: 80, lastReceivedAt: '' },
  },
  aggregate: { robotCount: 2, totalMsgCount: 180, totalRateHz: 17.6, totalKbps: 5.0, latencyMs: { mean: 30.1, p50: 29.0, p95: 55.0, p99: 65.0, max: 80.0 } },
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
      json: () => Promise.resolve({ robots: {}, aggregate: { robotCount: 0, totalMsgCount: 0, totalRateHz: 0, totalKbps: 0, latencyMs: { mean: 0, p50: 0, p95: 0, p99: 0, max: 0 } } }),
    })
    await act(async () => { wrap() })
    expect(screen.getByText(/No telemetry data yet/)).toBeTruthy()
  })
})

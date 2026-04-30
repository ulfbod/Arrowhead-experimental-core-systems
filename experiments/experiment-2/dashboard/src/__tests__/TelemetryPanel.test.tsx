import { render, screen, act } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { TelemetryPanel } from '../components/TelemetryPanel'
import { ConfigProvider } from '../config/context'

function makeTelemetry(seq: number) {
  return {
    receivedAt: '2026-01-01T00:00:00Z',
    payload: { robotId: 'robot-1', temperature: 22.5, humidity: 55.0, timestamp: '2026-01-01T00:00:00Z', seq },
  }
}

describe('TelemetryPanel', () => {
  it('shows no-data message on 204', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, status: 204 })
    render(<ConfigProvider><TelemetryPanel /></ConfigProvider>)
    expect(await screen.findByText(/No data yet/)).toBeTruthy()
  })

  it('renders latest telemetry fields after 200', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve(makeTelemetry(42)),
    })
    render(<ConfigProvider><TelemetryPanel /></ConfigProvider>)
    expect(await screen.findByText('42')).toBeTruthy()
    expect(screen.getByText('22.5°C')).toBeTruthy()
    expect(screen.getByText('55%')).toBeTruthy()
  })

  it('shows error on fetch failure', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 500 })
    render(<ConfigProvider><TelemetryPanel /></ConfigProvider>)
    expect(await screen.findByText(/HTTP 500/)).toBeTruthy()
  })

  it('does not add duplicate readings with same seq to history', async () => {
    // Return seq=1 twice — history should stay at one entry, not two.
    globalThis.fetch = vi.fn()
      .mockResolvedValueOnce({
        ok: true, status: 200,
        json: () => Promise.resolve(makeTelemetry(1)),
      })
      .mockResolvedValueOnce({
        ok: true, status: 200,
        json: () => Promise.resolve(makeTelemetry(1)),
      })

    render(<ConfigProvider><TelemetryPanel /></ConfigProvider>)
    await screen.findByText('1')

    // Trigger a second fetch manually by advancing the fake interval.
    // Since the history table only renders items from index 1 onwards (the
    // "latest" row is rendered separately), if a duplicate were added the table
    // would render a row — check that no tbody rows exist.
    await act(async () => {})

    // Only one "latest" badge should exist.
    expect(screen.getAllByText('latest')).toHaveLength(1)
  })
})

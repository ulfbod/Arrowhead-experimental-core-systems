import { render, screen, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import App from '../App'

// Stub every fetch so async polls resolve immediately and don't leave pending
// state updates after the test completes.
function stubFetch() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    if ((url as string).includes('/api/queues') || (url as string).includes('/api/exchanges')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) })
    }
    if ((url as string).includes('/telemetry/latest')) {
      return Promise.resolve({ ok: true, status: 204 })
    }
    if ((url as string).includes('/telemetry/stats') || (url as string).includes('/telemetry/all')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ robots: {}, aggregate: { robotCount: 0, totalMsgCount: 0, meanLatencyMs: 0, p95LatencyMs: 0 } }) })
    }
    if ((url as string).includes('/api/robot-fleet')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ payloadType: 'imu', payloadHz: 10, robots: [] }) })
    }
    if ((url as string).includes('/orchestration/dynamic')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ response: [] }) })
    }
    if ((url as string).includes('/health')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ status: 'ok' }) })
    }
    return Promise.resolve({
      ok: true, status: 200,
      json: () => Promise.resolve({ serviceQueryData: [], unfilteredHits: 0, rules: [], count: 0 }),
    })
  })
}

describe('App navigation', () => {
  beforeEach(stubFetch)

  it('defaults to Experiment 2 tab', async () => {
    render(<App />)
    await act(async () => {})
    expect(screen.getByRole('button', { name: 'Experiment 2' })).toHaveAttribute('aria-current', 'page')
  })

  it('renders experiment view content by default', async () => {
    render(<App />)
    await act(async () => {})
    expect(screen.getByText('Data Flow')).toBeTruthy()
    expect(screen.getByText('Live Telemetry')).toBeTruthy()
  })

  it('switches to Core Systems view on click', async () => {
    const user = userEvent.setup()
    render(<App />)
    await act(async () => {})
    await user.click(screen.getByRole('button', { name: 'Core Systems' }))
    await act(async () => {})
    expect(screen.getByRole('button', { name: 'Core Systems' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByText('System Health')).toBeTruthy()
  })

  it('switches to Support Systems view on click', async () => {
    const user = userEvent.setup()
    render(<App />)
    await act(async () => {})
    await user.click(screen.getByRole('button', { name: 'Support Systems' }))
    await act(async () => {})
    expect(screen.getByRole('button', { name: 'Support Systems' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByText('RabbitMQ')).toBeTruthy()
  })
})

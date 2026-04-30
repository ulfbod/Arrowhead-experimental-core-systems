import { render, screen } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { OrchestrationTrace } from '../components/OrchestrationTrace'
import { ConfigProvider } from '../config/context'

// No fake timers — findBy* relies on real setTimeout for waitFor retries.

function wrap(ui: React.ReactElement) {
  return render(<ConfigProvider>{ui}</ConfigProvider>)
}

describe('OrchestrationTrace', () => {
  it('shows no-providers message when response is empty', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({ response: [] }),
    })
    wrap(<OrchestrationTrace />)
    expect(await screen.findByText(/no providers found/)).toBeTruthy()
  })

  it('renders provider details when orchestration succeeds', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({
        response: [{
          provider: { systemName: 'edge-adapter', address: 'edge-adapter', port: 9001 },
          service: { serviceDefinition: 'telemetry', serviceUri: '/telemetry/latest', interfaces: ['HTTP-INSECURE-JSON'] },
        }],
      }),
    })
    wrap(<OrchestrationTrace />)
    // Wait for the unique port cell to appear (proves the table rendered).
    expect(await screen.findByText('9001')).toBeTruthy()
    // Provider and address columns both contain 'edge-adapter'.
    expect(screen.getAllByText('edge-adapter').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('/telemetry/latest')).toBeTruthy()
  })

  it('shows error when orchestration fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false, status: 403,
      text: () => Promise.resolve('forbidden'),
      json: () => Promise.reject(new Error()),
    })
    wrap(<OrchestrationTrace />)
    expect(await screen.findByText(/HTTP 403/)).toBeTruthy()
  })
})

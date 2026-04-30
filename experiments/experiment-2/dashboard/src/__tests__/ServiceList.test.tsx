import { render, screen } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { ServiceList } from '../components/ServiceList'

// No fake timers here — findBy* uses waitFor internally which relies on real
// setTimeout for retries; faking timers prevents those retries from firing.

describe('ServiceList', () => {
  it('shows loading initially', () => {
    globalThis.fetch = vi.fn().mockReturnValue(new Promise(() => {}))
    render(<ServiceList />)
    expect(screen.getByText(/loading/)).toBeTruthy()
  })

  it('renders service rows after data loads', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({
        serviceQueryData: [{
          id: 1,
          serviceDefinition: 'telemetry',
          providerSystem: { systemName: 'edge-adapter', address: 'edge-adapter', port: 9001 },
          serviceUri: '/telemetry/latest',
          interfaces: ['HTTP-INSECURE-JSON'],
          version: 1,
        }],
        unfilteredHits: 1,
      }),
    })
    render(<ServiceList />)
    expect(await screen.findByText('telemetry')).toBeTruthy()
    expect(screen.getByText('edge-adapter')).toBeTruthy()
  })

  it('shows empty message when no services', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({ serviceQueryData: [], unfilteredHits: 0 }),
    })
    render(<ServiceList />)
    expect(await screen.findByText(/No services/)).toBeTruthy()
  })

  it('shows error message on fetch failure', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false, status: 503,
      text: () => Promise.resolve('service unavailable'),
      json: () => Promise.reject(new Error()),
    })
    render(<ServiceList />)
    expect(await screen.findByText(/HTTP 503/)).toBeTruthy()
  })
})

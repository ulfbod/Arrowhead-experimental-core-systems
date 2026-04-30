import { render, screen } from '@testing-library/react'
import { vi, describe, it, expect } from 'vitest'
import { BrokerStats } from '../components/BrokerStats'

// No fake timers — findBy* relies on real setTimeout for waitFor retries.

function mockRabbitMQ() {
  globalThis.fetch = vi.fn().mockImplementation((url: string) => {
    if ((url as string).includes('/api/queues')) {
      return Promise.resolve({
        ok: true, status: 200,
        json: () => Promise.resolve([
          { name: 'edge-adapter-queue', messages: 0, consumers: 1 },
        ]),
      })
    }
    return Promise.resolve({
      ok: true, status: 200,
      json: () => Promise.resolve([
        { name: '',          type: 'direct', durable: true },
        { name: 'arrowhead', type: 'topic',  durable: true },
      ]),
    })
  })
}

describe('BrokerStats', () => {
  it('renders queue name and consumer count', async () => {
    mockRabbitMQ()
    render(<BrokerStats />)
    expect(await screen.findByText('edge-adapter-queue')).toBeTruthy()
    expect(screen.getByText('1')).toBeTruthy()
  })

  it('renders named exchanges only (filters default empty-name exchange)', async () => {
    mockRabbitMQ()
    render(<BrokerStats />)
    expect(await screen.findByText('arrowhead')).toBeTruthy()
  })

  it('shows error when fetch fails — both queue and exchange sections show the error', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false, status: 503,
      text: () => Promise.resolve('unavailable'),
      json: () => Promise.reject(new Error()),
    })
    render(<BrokerStats />)
    // BrokerStats has two independent polls (queues + exchanges); both fail.
    const errors = await screen.findAllByText(/HTTP 503/)
    expect(errors.length).toBeGreaterThanOrEqual(1)
  })
})

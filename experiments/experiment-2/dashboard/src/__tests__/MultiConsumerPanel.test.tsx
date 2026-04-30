import { render, screen, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { MultiConsumerPanel } from '../components/MultiConsumerPanel'
import { ConfigProvider } from '../config/context'

function wrap() {
  return render(
    <ConfigProvider>
      <MultiConsumerPanel />
    </ConfigProvider>,
  )
}

describe('MultiConsumerPanel', () => {
  beforeEach(() => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true, status: 200,
      json: () => Promise.resolve({ response: [{ provider: { systemName: 'edge-adapter', address: 'localhost', port: 9001 }, service: { serviceDefinition: 'telemetry', serviceUri: '/telemetry/latest', interfaces: ['HTTP-INSECURE-JSON'] } }] }),
    })
  })

  it('renders section with testid', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('multi-consumer-panel')).toBeTruthy()
  })

  it('renders heading', async () => {
    await act(async () => { wrap() })
    expect(screen.getByText('Consumer Status')).toBeTruthy()
  })

  it('renders card for demo-consumer', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('consumer-card-demo-consumer')).toBeTruthy()
  })

  it('renders card for demo-consumer-2', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('consumer-card-demo-consumer-2')).toBeTruthy()
  })

  it('renders card for demo-consumer-3', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('consumer-card-demo-consumer-3')).toBeTruthy()
  })

  it('shows consumer labels', async () => {
    await act(async () => { wrap() })
    expect(screen.getByText('Consumer 1')).toBeTruthy()
    expect(screen.getByText('Consumer 2')).toBeTruthy()
    expect(screen.getByText('Consumer 3')).toBeTruthy()
  })
})

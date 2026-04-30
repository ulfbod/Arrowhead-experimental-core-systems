import { render, screen, fireEvent, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach } from 'vitest'
import { SimulationControl } from '../components/SimulationControl'
import { ConfigProvider } from '../config/context'

const defaultConfig: import('../types').FleetConfig = {
  payloadType: 'imu',
  payloadHz: 10,
  robots: [{ id: 'robot-1', networkPreset: '5g_good' }],
}

function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
  })
}

function wrap() {
  return render(
    <ConfigProvider>
      <SimulationControl />
    </ConfigProvider>,
  )
}

describe('SimulationControl', () => {
  beforeEach(() => {
    globalThis.fetch = mockFetch(200, defaultConfig)
  })

  it('renders heading', async () => {
    await act(async () => { wrap() })
    expect(screen.getByText('Fleet Simulation Control')).toBeTruthy()
  })

  it('renders robot count input', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('robot-count')).toBeTruthy()
  })

  it('renders payload type select', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('payload-type')).toBeTruthy()
  })

  it('renders network preset select', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('network-preset')).toBeTruthy()
  })

  it('renders Apply button', async () => {
    await act(async () => { wrap() })
    expect(screen.getByTestId('apply-btn')).toBeTruthy()
  })

  it('Apply calls POST with new config', async () => {
    let postedBody: unknown
    globalThis.fetch = vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        postedBody = JSON.parse(init.body as string)
        return Promise.resolve({ ok: true, status: 204, text: () => Promise.resolve('') })
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(defaultConfig), text: () => Promise.resolve('') })
    })

    await act(async () => { wrap() })

    const countInput = screen.getByTestId('robot-count') as HTMLInputElement
    fireEvent.change(countInput, { target: { value: '5' } })

    await act(async () => {
      fireEvent.click(screen.getByTestId('apply-btn'))
    })

    expect(postedBody).toBeTruthy()
    const cfg = postedBody as import('../types').FleetConfig
    expect(cfg.robots).toHaveLength(5)
  })

  it('network preset select contains 5g options', async () => {
    await act(async () => { wrap() })
    const sel = screen.getByTestId('network-preset') as HTMLSelectElement
    const options = Array.from(sel.options).map(o => o.value)
    expect(options).toContain('5g_good')
    expect(options).toContain('5g_excellent')
    expect(options).toContain('5g_poor')
  })
})

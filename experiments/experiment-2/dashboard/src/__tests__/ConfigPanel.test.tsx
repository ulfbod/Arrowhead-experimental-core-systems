import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, beforeEach } from 'vitest'
import { ConfigPanel } from '../components/ConfigPanel'
import { ConfigProvider } from '../config/context'
import { DEFAULT_CONFIG } from '../config/defaults'

beforeEach(() => localStorage.clear())

function wrap() {
  return render(
    <ConfigProvider>
      <ConfigPanel />
    </ConfigProvider>,
  )
}

describe('ConfigPanel', () => {
  it('renders all three section headings', () => {
    wrap()
    expect(screen.getByText('Polling intervals')).toBeTruthy()
    expect(screen.getByText('Display')).toBeTruthy()
    expect(screen.getByText('Experiment 2')).toBeTruthy()
  })

  it('shows default health interval', () => {
    wrap()
    const input = screen.getByLabelText(/Health check/i) as HTMLInputElement
    expect(Number(input.value)).toBe(DEFAULT_CONFIG.polling.healthIntervalMs)
  })

  it('shows default consumer name', () => {
    wrap()
    const input = screen.getByLabelText(/Consumer system name/i) as HTMLInputElement
    expect(input.value).toBe(DEFAULT_CONFIG.experiment2.consumerName)
  })

  it('Apply button is present', () => {
    wrap()
    expect(screen.getByRole('button', { name: 'Apply' })).toBeTruthy()
  })

  it('Reset to defaults button is present', () => {
    wrap()
    expect(screen.getByRole('button', { name: 'Reset to defaults' })).toBeTruthy()
  })

  it('typing in consumer name field updates the draft', () => {
    wrap()
    const input = screen.getByLabelText(/Consumer system name/i) as HTMLInputElement
    fireEvent.change(input, { target: { value: 'my-consumer' } })
    expect(input.value).toBe('my-consumer')
  })

  it('shows fleet stats interval input', () => {
    wrap()
    const input = screen.getByLabelText(/Fleet stats/i) as HTMLInputElement
    expect(Number(input.value)).toBe(DEFAULT_CONFIG.polling.fleetStatsIntervalMs)
  })

  it('shows telemetry stats interval input', () => {
    wrap()
    const input = screen.getByLabelText(/Telemetry stats/i) as HTMLInputElement
    expect(Number(input.value)).toBe(DEFAULT_CONFIG.polling.allTelemetryIntervalMs)
  })
})

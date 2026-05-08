// Tests for ConfigProvider and useConfig.
//
// storage.ts is experiment-specific (not part of dashboard-shared); it is
// mocked here so context.tsx can be tested in isolation.

import { render, screen, act } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { ConfigProvider, useConfig } from './context'
import { DEFAULT_CONFIG } from './defaults'
import type { DashboardConfig } from './types'

vi.mock('./storage', () => ({
  loadConfig:  vi.fn(() => DEFAULT_CONFIG),
  saveConfig:  vi.fn(),
  resetConfig: vi.fn(() => DEFAULT_CONFIG),
}))

// ── Helpers ───────────────────────────────────────────────────────────────────

function ConfigConsumer({ onRender }: { onRender: (cfg: DashboardConfig) => void }) {
  const { config } = useConfig()
  onRender(config)
  return <div data-testid="consumer">{config.polling.healthIntervalMs}</div>
}

// ── useConfig outside provider ────────────────────────────────────────────────

describe('useConfig — outside provider', () => {
  it('throws when rendered outside <ConfigProvider>', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<ConfigConsumer onRender={() => {}} />)).toThrow(
      'useConfig must be used inside <ConfigProvider>',
    )
    spy.mockRestore()
  })
})

// ── ConfigProvider — initial value ────────────────────────────────────────────

describe('ConfigProvider — initial value', () => {
  it('provides the config returned by loadConfig', () => {
    let received: DashboardConfig | null = null
    render(
      <ConfigProvider>
        <ConfigConsumer onRender={cfg => { received = cfg }} />
      </ConfigProvider>,
    )
    expect(received).toEqual(DEFAULT_CONFIG)
  })

  it('renders children', () => {
    render(
      <ConfigProvider>
        <ConfigConsumer onRender={() => {}} />
      </ConfigProvider>,
    )
    expect(screen.getByTestId('consumer')).toBeInTheDocument()
  })
})

// ── setConfig ─────────────────────────────────────────────────────────────────

describe('useConfig — setConfig', () => {
  it('updates the config available to consumers', () => {
    const updated: DashboardConfig = {
      ...DEFAULT_CONFIG,
      polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 99_000 },
    }

    function Setter() {
      const { config, setConfig } = useConfig()
      return (
        <>
          <div data-testid="val">{config.polling.healthIntervalMs}</div>
          <button onClick={() => setConfig(updated)}>update</button>
        </>
      )
    }

    render(<ConfigProvider><Setter /></ConfigProvider>)
    expect(screen.getByTestId('val')).toHaveTextContent(
      String(DEFAULT_CONFIG.polling.healthIntervalMs),
    )

    act(() => { screen.getByRole('button', { name: 'update' }).click() })
    expect(screen.getByTestId('val')).toHaveTextContent('99000')
  })
})

// ── resetToDefaults ───────────────────────────────────────────────────────────

describe('useConfig — resetToDefaults', () => {
  it('restores DEFAULT_CONFIG after a setConfig call', () => {
    const custom: DashboardConfig = {
      ...DEFAULT_CONFIG,
      polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 1_000 },
    }

    function Resetter() {
      const { config, setConfig, resetToDefaults } = useConfig()
      return (
        <>
          <div data-testid="val">{config.polling.healthIntervalMs}</div>
          <button data-testid="set"   onClick={() => setConfig(custom)}>set</button>
          <button data-testid="reset" onClick={resetToDefaults}>reset</button>
        </>
      )
    }

    render(<ConfigProvider><Resetter /></ConfigProvider>)

    act(() => { screen.getByTestId('set').click() })
    expect(screen.getByTestId('val')).toHaveTextContent('1000')

    act(() => { screen.getByTestId('reset').click() })
    expect(screen.getByTestId('val')).toHaveTextContent(
      String(DEFAULT_CONFIG.polling.healthIntervalMs),
    )
  })
})

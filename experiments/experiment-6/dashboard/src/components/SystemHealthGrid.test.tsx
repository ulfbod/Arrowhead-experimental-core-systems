// Unit tests for SystemHealthGrid — the experiment-6-specific health grid.
//
// SystemHealthGrid hard-codes the 19 services that make up the experiment-6 stack
// and organises them by layer (core / support / experiment).  These tests document:
//   - the expected service count and layer legend
//   - the probe-status display rules (loading → no label, down → "down" label)
//   - the showHealthLatency config flag
//
// Both usePolling and useConfig are mocked so tests run without a live server.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { SystemHealthGrid } from './SystemHealthGrid'
import { usePolling } from '../hooks/usePolling'
import { useConfig } from '../config/context'

vi.mock('../hooks/usePolling')
vi.mock('../config/context')
vi.mock('../api', () => ({
  fetchHealthProbe:  vi.fn(),
  fetchRabbitMQHealth: vi.fn(),
}))

const mockUsePolling = vi.mocked(usePolling)
const mockUseConfig  = vi.mocked(useConfig)

function noData() {
  return { data: null, error: null, loading: true, stale: false, refresh: vi.fn() }
}

// Minimal config object that satisfies SystemHealthGrid's useConfig reads.
function makeConfig(overrides: { showHealthLatency?: boolean } = {}) {
  return {
    config: {
      polling:  { healthIntervalMs: 5000, grantsIntervalMs: 5000, rmqUsersIntervalMs: 5000, consumerStatsIntervalMs: 3000, policyIntervalMs: 5000 },
      display:  { showHealthLatency: overrides.showHealthLatency ?? false },
    },
    setConfig:      vi.fn(),
    resetToDefaults: vi.fn(),
  }
}

beforeEach(() => {
  // Each SystemCard calls usePolling once; 19 cards = 19 calls.
  mockUsePolling.mockReturnValue(noData())
  mockUseConfig.mockReturnValue(makeConfig() as ReturnType<typeof useConfig>)
})

// ── Card count and labels ─────────────────────────────────────────────────────

describe('SystemHealthGrid — card rendering', () => {
  it('renders exactly 19 health cards (5 core + 3 support + 11 experiment)', () => {
    render(<SystemHealthGrid />)
    const cards = document.querySelectorAll('[data-testid^="health-card-"]')
    expect(cards.length).toBe(19)
  })

  it('renders a card for every layer — spot-checks across core, support, experiment', () => {
    render(<SystemHealthGrid />)
    expect(document.querySelector('[data-testid="health-card-serviceregistry"]')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="health-card-rabbitmq"]')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="health-card-rest-consumer"]')).toBeInTheDocument()
  })

  it('renders a human-readable label for each card', () => {
    render(<SystemHealthGrid />)
    expect(screen.getByText('ServiceRegistry')).toBeInTheDocument()
    expect(screen.getByText('RabbitMQ')).toBeInTheDocument()
    expect(screen.getByText('RestConsumer')).toBeInTheDocument()
  })
})

// ── Layer legend ──────────────────────────────────────────────────────────────

describe('SystemHealthGrid — layer legend', () => {
  it('renders the three layer legend items', () => {
    render(<SystemHealthGrid />)
    expect(screen.getByText('core')).toBeInTheDocument()
    expect(screen.getByText('support')).toBeInTheDocument()
    expect(screen.getByText('experiment')).toBeInTheDocument()
  })
})

// ── Probe status display ──────────────────────────────────────────────────────

describe('SystemHealthGrid — probe status', () => {
  it('shows "down" label when probe.status is "down"', () => {
    mockUsePolling.mockReturnValue({
      data:    { status: 'down', latencyMs: 0, error: 'connection refused' },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<SystemHealthGrid />)
    // Every card gets the same probe; multiple "down" labels appear
    expect(screen.getAllByText('down').length).toBeGreaterThanOrEqual(1)
  })

  it('shows latency in ms when showHealthLatency is true and latencyMs > 0', () => {
    mockUseConfig.mockReturnValue(makeConfig({ showHealthLatency: true }) as ReturnType<typeof useConfig>)
    mockUsePolling.mockReturnValue({
      data:    { status: 'ok', latencyMs: 42 },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<SystemHealthGrid />)
    expect(screen.getAllByText('42ms').length).toBeGreaterThanOrEqual(1)
  })

  it('hides latency when showHealthLatency is false even if latencyMs > 0', () => {
    mockUseConfig.mockReturnValue(makeConfig({ showHealthLatency: false }) as ReturnType<typeof useConfig>)
    mockUsePolling.mockReturnValue({
      data:    { status: 'ok', latencyMs: 42 },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<SystemHealthGrid />)
    expect(screen.queryByText('42ms')).not.toBeInTheDocument()
  })

  it('does not render "down" label when all probes are ok', () => {
    mockUsePolling.mockReturnValue({
      data:    { status: 'ok', latencyMs: 5 },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<SystemHealthGrid />)
    expect(screen.queryByText('down')).not.toBeInTheDocument()
  })
})

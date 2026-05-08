// Unit tests for ConsumerStatsPanel — live message counts across all three transports.
//
// ConsumerStatsPanel makes 6 usePolling calls in a fixed order:
//   1. fetchQueues            → queue depth + deliver rates for the AMQP queue map
//   2. consumer-1 stats       → AmqpConsumerCard (Consumer-1)
//   3. consumer-2 stats       → AmqpConsumerCard (Consumer-2)
//   4. consumer-3 stats       → AmqpConsumerCard (Consumer-3)
//   5. fetchAnalyticsStats    → AnalyticsConsumerCard (Kafka/SSE)
//   6. fetchRestConsumerStats → RestConsumerCard (REST/HTTP)
//
// This ordering is load-bearing for mockReturnValueOnce chains — do not reorder.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ConsumerStatsPanel } from './ConsumerStatsPanel'
import { usePolling } from '../hooks/usePolling'
import type { ConsumerStats, AnalyticsStats, RestConsumerStats, RabbitQueue } from '../types'

vi.mock('../hooks/usePolling')
vi.mock('../api', () => ({
  fetchConsumerStats:      vi.fn(),
  fetchQueues:             vi.fn(),
  fetchAnalyticsStats:     vi.fn(),
  fetchRestConsumerStats:  vi.fn(),
}))

const mockUsePolling = vi.mocked(usePolling)

function noData() {
  return { data: null, error: null, loading: false, stale: false, refresh: vi.fn() }
}

function withData<T>(data: T) {
  return { data, error: null, loading: false, stale: false, refresh: vi.fn() }
}

function withError() {
  return { data: null, error: 'unavailable', loading: false, stale: false, refresh: vi.fn() }
}

// Set up all 6 usePolling mock return values in the order the component calls them.
function setup({
  queues   = noData(),
  c1       = noData(),
  c2       = noData(),
  c3       = noData(),
  analytics = noData(),
  rest     = noData(),
} = {}) {
  mockUsePolling
    .mockReturnValueOnce(queues   as ReturnType<typeof usePolling>)
    .mockReturnValueOnce(c1       as ReturnType<typeof usePolling>)
    .mockReturnValueOnce(c2       as ReturnType<typeof usePolling>)
    .mockReturnValueOnce(c3       as ReturnType<typeof usePolling>)
    .mockReturnValueOnce(analytics as ReturnType<typeof usePolling>)
    .mockReturnValueOnce(rest     as ReturnType<typeof usePolling>)
}

beforeEach(() => {
  // Default: all 6 calls return noData
  mockUsePolling.mockReturnValue(noData())
})

// ── Static structure ──────────────────────────────────────────────────────────

describe('ConsumerStatsPanel — static structure', () => {
  it('renders the "Live Consumer Data" heading', () => {
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('Live Consumer Data')).toBeInTheDocument()
  })

  it('renders labels for all three AMQP consumers', () => {
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('Consumer-1')).toBeInTheDocument()
    expect(screen.getByText('Consumer-2')).toBeInTheDocument()
    expect(screen.getByText('Consumer-3')).toBeInTheDocument()
  })

  it('renders labels for the Kafka analytics and REST consumers', () => {
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('Analytics Consumer')).toBeInTheDocument()
    expect(screen.getByText('REST Consumer')).toBeInTheDocument()
  })

  it('shows the transport label for each card', () => {
    render(<ConsumerStatsPanel />)
    expect(screen.getAllByText('AMQP / RabbitMQ').length).toBe(3)
    expect(screen.getByText('Kafka / SSE')).toBeInTheDocument()
    expect(screen.getByText(/REST \/ HTTP/)).toBeInTheDocument()
  })
})

// ── Loading placeholders ──────────────────────────────────────────────────────

describe('ConsumerStatsPanel — loading state', () => {
  it('shows "…" placeholder for all stat values when no data has loaded', () => {
    render(<ConsumerStatsPanel />)
    // Each of the 5 consumer cards shows at least one "…"
    const ellipses = screen.getAllByText('…')
    expect(ellipses.length).toBeGreaterThanOrEqual(5)
  })
})

// ── AMQP consumer data ────────────────────────────────────────────────────────

describe('ConsumerStatsPanel — AMQP consumer data', () => {
  it('shows msgCount from Consumer-1 stats', () => {
    const c1Stats: ConsumerStats = { name: 'consumer-1', msgCount: 42, lastReceivedAt: '' }
    setup({ c1: withData(c1Stats) })
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('42')).toBeInTheDocument()
  })

  it('shows deliver rate from RabbitMQ queue data as "N.N msg/s"', () => {
    const queues: RabbitQueue[] = [{
      name: 'demo-consumer-1-queue',
      messages: 3,
      consumers: 1,
      message_stats: { deliver_details: { rate: 2.5 } },
    }]
    setup({ queues: withData(queues) })
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('2.5 msg/s')).toBeInTheDocument()
  })

  it('shows queue depth from RabbitMQ queue data', () => {
    const queues: RabbitQueue[] = [{
      name: 'demo-consumer-2-queue',
      messages: 7,
      consumers: 1,
    }]
    setup({ queues: withData(queues) })
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('7')).toBeInTheDocument()
  })

  it('shows "unavailable" for an AMQP card that has an error', () => {
    setup({ c1: withError() })
    render(<ConsumerStatsPanel />)
    expect(screen.getAllByText('unavailable').length).toBeGreaterThanOrEqual(1)
  })
})

// ── Analytics consumer (Kafka/SSE) ───────────────────────────────────────────

describe('ConsumerStatsPanel — analytics consumer', () => {
  it('shows analytics msgCount', () => {
    const stats: AnalyticsStats = { name: 'analytics-consumer', msgCount: 99, lastReceivedAt: '2024-01-01T10:00:00Z', transport: 'kafka', lastDeniedAt: '' }
    setup({ analytics: withData(stats) })
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('99')).toBeInTheDocument()
  })

  it('shows "—" for lastDeniedAt when it is empty', () => {
    const stats: AnalyticsStats = { name: 'analytics-consumer', msgCount: 1, lastReceivedAt: '2024-01-01T10:00:00Z', transport: 'kafka', lastDeniedAt: '' }
    setup({ analytics: withData(stats) })
    render(<ConsumerStatsPanel />)
    // formatTime('') returns '—'
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1)
  })
})

// ── REST consumer ─────────────────────────────────────────────────────────────

describe('ConsumerStatsPanel — REST consumer', () => {
  it('shows REST consumer msgCount and deniedCount', () => {
    const stats: RestConsumerStats = {
      name: 'rest-consumer',
      msgCount: 5,
      deniedCount: 2,
      lastReceivedAt: '2024-01-01T10:00:00Z',
      lastDeniedAt:   '',
      transport: 'rest',
    }
    setup({ rest: withData(stats) })
    render(<ConsumerStatsPanel />)
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('shows "unavailable" for the REST card when it has an error', () => {
    setup({ rest: withError() })
    render(<ConsumerStatsPanel />)
    expect(screen.getAllByText('unavailable').length).toBeGreaterThanOrEqual(1)
  })
})

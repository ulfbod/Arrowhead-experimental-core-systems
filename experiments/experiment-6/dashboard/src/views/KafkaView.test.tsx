// Unit tests for KafkaView — the Kafka transport monitoring tab.

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { KafkaView } from './KafkaView'
import { usePolling } from '../hooks/usePolling'
import { checkKafkaAuthz } from '../api'

vi.mock('../hooks/usePolling')
vi.mock('../api', () => ({
  fetchKafkaAuthzStatus: vi.fn(),
  checkKafkaAuthz: vi.fn(),
}))

const mockUsePolling     = vi.mocked(usePolling)
const mockCheckKafkaAuthz = vi.mocked(checkKafkaAuthz)

function noData() {
  return { data: null, error: null, loading: false, stale: false, refresh: vi.fn() }
}

beforeEach(() => {
  mockUsePolling.mockReturnValue(noData())
  mockCheckKafkaAuthz.mockResolvedValue({
    consumer: 'analytics-consumer',
    service:  'telemetry',
    permit:   true,
    decision: 'Permit',
  })
})

// ── Static structure ──────────────────────────────────────────────────────────

describe('KafkaView — static structure', () => {
  it('renders the heading', () => {
    render(<KafkaView />)
    expect(screen.getByText('Kafka Transport Monitor')).toBeInTheDocument()
  })

  it('renders the Live Status section heading', () => {
    render(<KafkaView />)
    expect(screen.getByText('Live Status')).toBeInTheDocument()
  })

  it('renders the Authorization Check section heading', () => {
    render(<KafkaView />)
    expect(screen.getByText('Authorization Check')).toBeInTheDocument()
  })

  it('renders the Architecture section heading', () => {
    render(<KafkaView />)
    expect(screen.getByText('Kafka Path Architecture')).toBeInTheDocument()
  })

  it('renders the kafka-authz card title', () => {
    render(<KafkaView />)
    // "kafka-authz" appears in both the intro paragraph and the card title
    expect(screen.getAllByText('kafka-authz').length).toBeGreaterThanOrEqual(1)
  })
})

// ── Status card ───────────────────────────────────────────────────────────────

describe('KafkaView — status card', () => {
  it('shows live stream count and total served when data is available', () => {
    mockUsePolling.mockReturnValue({
      data:    { activeStreams: 3, totalServed: 247 },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<KafkaView />)
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('247')).toBeInTheDocument()
  })

  it('shows ellipsis placeholder while data is loading', () => {
    mockUsePolling.mockReturnValue(noData())
    render(<KafkaView />)
    // Both "active streams" and "total served" values show '…' when data is null
    const ellipses = screen.getAllByText('…')
    expect(ellipses.length).toBeGreaterThanOrEqual(2)
  })
})

// ── Authorization check form ──────────────────────────────────────────────────

describe('KafkaView — auth check form', () => {
  it('renders consumer and service input fields and a submit button', () => {
    render(<KafkaView />)
    expect(screen.getByPlaceholderText('consumer name')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('service definition')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Check Authorization' })).toBeInTheDocument()
  })

  it('pre-fills consumer with "analytics-consumer" and service with "telemetry"', () => {
    render(<KafkaView />)
    expect(screen.getByPlaceholderText('consumer name')).toHaveValue('analytics-consumer')
    expect(screen.getByPlaceholderText('service definition')).toHaveValue('telemetry')
  })

  it('calls checkKafkaAuthz with entered values on submit', async () => {
    render(<KafkaView />)
    fireEvent.click(screen.getByRole('button', { name: 'Check Authorization' }))
    await waitFor(() => expect(mockCheckKafkaAuthz).toHaveBeenCalledWith('analytics-consumer', 'telemetry'))
  })

  it('shows a Permit badge after a successful Permit decision', async () => {
    mockCheckKafkaAuthz.mockResolvedValue({
      consumer: 'analytics-consumer', service: 'telemetry',
      permit: true, decision: 'Permit',
    })
    render(<KafkaView />)
    fireEvent.click(screen.getByRole('button', { name: 'Check Authorization' }))
    await screen.findByText('Permit')
  })

  it('shows a Deny badge after a Deny decision', async () => {
    mockCheckKafkaAuthz.mockResolvedValue({
      consumer: 'unknown', service: 'telemetry',
      permit: false, decision: 'Deny',
    })
    render(<KafkaView />)
    const consumerInput = screen.getByPlaceholderText('consumer name')
    fireEvent.change(consumerInput, { target: { value: 'unknown' } })
    fireEvent.click(screen.getByRole('button', { name: 'Check Authorization' }))
    await screen.findByText('Deny')
  })

  it('shows an error box when checkKafkaAuthz rejects', async () => {
    mockCheckKafkaAuthz.mockRejectedValue(new Error('PDP unavailable'))
    render(<KafkaView />)
    fireEvent.click(screen.getByRole('button', { name: 'Check Authorization' }))
    await screen.findByText(/PDP unavailable/)
  })

  it('chip buttons fill the consumer input', () => {
    render(<KafkaView />)
    fireEvent.click(screen.getByRole('button', { name: 'demo-consumer-1' }))
    expect(screen.getByPlaceholderText('consumer name')).toHaveValue('demo-consumer-1')
  })
})

// Tests for the links added to PolicyProjectionPanel in the AMQP and Kafka cards.

import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { PolicyProjectionPanel } from './PolicyProjectionPanel'
import { usePolling } from '../hooks/usePolling'

vi.mock('../hooks/usePolling')
vi.mock('../api', () => ({
  fetchPolicySyncStatus:  vi.fn(),
  fetchKafkaAuthzStatus:  vi.fn(),
  fetchRestAuthzStatus:   vi.fn(),
}))

const mockUsePolling = vi.mocked(usePolling)

beforeEach(() => {
  mockUsePolling.mockReturnValue({
    data: null, error: null, loading: false, stale: false, refresh: vi.fn(),
  })
})

// ── RabbitMQ link ─────────────────────────────────────────────────────────────

describe('PolicyProjectionPanel — RabbitMQ link', () => {
  it('renders a link to the RabbitMQ management UI', () => {
    render(<PolicyProjectionPanel />)
    const link = screen.getByRole('link', { name: /localhost:15676/ })
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute('href', 'http://localhost:15676')
  })

  it('opens the link in a new tab', () => {
    render(<PolicyProjectionPanel />)
    const link = screen.getByRole('link', { name: /localhost:15676/ })
    expect(link).toHaveAttribute('target', '_blank')
  })

  it('shows the admin credentials next to the link', () => {
    render(<PolicyProjectionPanel />)
    expect(screen.getByText('admin / admin')).toBeInTheDocument()
  })
})

// ── Kafka tab pointer ─────────────────────────────────────────────────────────

describe('PolicyProjectionPanel — Kafka tab pointer', () => {
  it('shows the Kafka tab pointer in the Kafka card', () => {
    render(<PolicyProjectionPanel />)
    expect(screen.getByText('→ Kafka tab')).toBeInTheDocument()
  })

  it('explains that no external UI is available for Kafka', () => {
    render(<PolicyProjectionPanel />)
    expect(screen.getByText(/No external UI/)).toBeInTheDocument()
  })
})

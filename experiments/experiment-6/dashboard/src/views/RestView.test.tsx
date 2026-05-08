// Unit tests for RestView — the REST/HTTP tab showing the Arrowhead core perspective.

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { RestView } from './RestView'
import { usePolling } from '../hooks/usePolling'
import { fetchThroughRestAuthz } from '../api'

vi.mock('../hooks/usePolling')
vi.mock('../api', () => ({
  fetchRestAuthzStatus:   vi.fn(),
  fetchDataProviderStats: vi.fn(),
  fetchRestConsumerStats: vi.fn(),
  fetchAuthRules:         vi.fn(),
  queryServiceRegistry:   vi.fn(),
  fetchThroughRestAuthz:  vi.fn(),
}))

const mockUsePolling          = vi.mocked(usePolling)
const mockFetchThroughRestAuthz = vi.mocked(fetchThroughRestAuthz)

function noData() {
  return { data: null, error: null, loading: false, stale: false, refresh: vi.fn() }
}

beforeEach(() => {
  // RestView calls usePolling 5 times; default all to no-data.
  mockUsePolling.mockReturnValue(noData())
  mockFetchThroughRestAuthz.mockResolvedValue({ status: 200, body: '{"robotId":"robot-1","temp":22}' })
})

// ── Static structure ──────────────────────────────────────────────────────────

describe('RestView — static structure', () => {
  it('renders the main heading', () => {
    render(<RestView />)
    expect(screen.getByText(/REST \/ HTTP — Arrowhead Core Perspective/)).toBeInTheDocument()
  })

  it('renders the Service Registration section', () => {
    render(<RestView />)
    expect(screen.getByText(/① Service Registration/)).toBeInTheDocument()
  })

  it('renders the Authorization Grants section', () => {
    render(<RestView />)
    expect(screen.getByText(/② Authorization Grants/)).toBeInTheDocument()
  })

  it('renders the Enforcement & Data Flow section', () => {
    render(<RestView />)
    expect(screen.getByText(/③ Enforcement/)).toBeInTheDocument()
  })

  it('renders the Live Authorization Demo section', () => {
    render(<RestView />)
    expect(screen.getByText(/④ Live Authorization Demo/)).toBeInTheDocument()
  })

  it('renders the architecture diagram with key flow labels', () => {
    render(<RestView />)
    const pre = document.querySelector('pre')
    expect(pre?.textContent).toContain('REST/HTTP lifecycle')
    expect(pre?.textContent).toContain('ServiceRegistry')
    expect(pre?.textContent).toContain('ConsumerAuthorization')
    expect(pre?.textContent).toContain('rest-authz')
    expect(pre?.textContent).toContain('Permit')
    expect(pre?.textContent).toContain('Deny')
  })
})

// ── Registered services section ───────────────────────────────────────────────

describe('RestView — registered services', () => {
  it('shows "no instances" message when SR returns empty', () => {
    mockUsePolling
      .mockReturnValueOnce(noData())                                               // restStatus
      .mockReturnValueOnce(noData())                                               // dpStats
      .mockReturnValueOnce(noData())                                               // rcStats
      .mockReturnValueOnce(noData())                                               // caRules
      .mockReturnValueOnce({ data: { serviceQueryData: [], unfilteredHits: 0 }, error: null, loading: false, stale: false, refresh: vi.fn() }) // srResult
    render(<RestView />)
    expect(screen.getByText(/No instances of/)).toBeInTheDocument()
  })

  it('renders a table row when SR returns a registered instance', () => {
    const srResult = {
      serviceQueryData: [{
        id: 1,
        serviceDefinition: 'telemetry-rest',
        providerSystem: { systemName: 'data-provider', address: 'data-provider', port: 9094 },
        serviceUri: '/telemetry/latest',
        interfaces: ['HTTP-INSECURE-JSON'],
        version: 1,
      }],
      unfilteredHits: 1,
    }
    mockUsePolling
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce({ data: srResult, error: null, loading: false, stale: false, refresh: vi.fn() })
    render(<RestView />)
    // systemName and address both equal "data-provider" — two cells
    expect(screen.getAllByText('data-provider').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('/telemetry/latest')).toBeInTheDocument()
    expect(screen.getByText('HTTP-INSECURE-JSON')).toBeInTheDocument()
  })
})

// ── Authorization grants section ──────────────────────────────────────────────

describe('RestView — authorization grants', () => {
  it('filters CA rules to show only telemetry-rest grants', () => {
    const caRules = {
      rules: [
        { id: 1, consumerSystemName: 'rest-consumer',   providerSystemName: 'data-provider', serviceDefinition: 'telemetry-rest' },
        { id: 2, consumerSystemName: 'demo-consumer-1', providerSystemName: 'robot-fleet',   serviceDefinition: 'telemetry' },
      ],
      count: 2,
    }
    mockUsePolling
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce({ data: caRules, error: null, loading: false, stale: false, refresh: vi.fn() })
      .mockReturnValueOnce(noData())
    render(<RestView />)
    // Grant table row for rest-consumer should appear (in a <td> element)
    const tds = document.querySelectorAll('td')
    const tdTexts = Array.from(tds).map(td => td.textContent)
    expect(tdTexts).toContain('rest-consumer')
    // demo-consumer-1 is for 'telemetry', not 'telemetry-rest' — must NOT appear in any <td>
    expect(tdTexts).not.toContain('demo-consumer-1')
  })

  it('shows "no grants" message when no telemetry-rest grants exist', () => {
    const caRules = {
      rules: [
        { id: 1, consumerSystemName: 'demo-consumer-1', providerSystemName: 'robot-fleet', serviceDefinition: 'telemetry' },
      ],
      count: 1,
    }
    mockUsePolling
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce(noData())
      .mockReturnValueOnce({ data: caRules, error: null, loading: false, stale: false, refresh: vi.fn() })
      .mockReturnValueOnce(noData())
    render(<RestView />)
    expect(screen.getByText(/No grants for/)).toBeInTheDocument()
  })
})

// ── Live authorization demo ───────────────────────────────────────────────────

describe('RestView — live authorization demo', () => {
  it('renders the Fetch button and consumer selector', () => {
    render(<RestView />)
    expect(screen.getByRole('button', { name: /Fetch via rest-authz/ })).toBeInTheDocument()
    // Both a <select> and a free-text <input> start with 'rest-consumer'
    expect(screen.getAllByDisplayValue('rest-consumer').length).toBeGreaterThanOrEqual(1)
  })

  it('calls fetchThroughRestAuthz with the selected consumer on submit', async () => {
    render(<RestView />)
    fireEvent.click(screen.getByRole('button', { name: /Fetch via rest-authz/ }))
    await waitFor(() => expect(mockFetchThroughRestAuthz).toHaveBeenCalledWith('rest-consumer'))
  })

  it('shows 200 OK Permit badge and JSON body after authorized fetch', async () => {
    mockFetchThroughRestAuthz.mockResolvedValue({
      status: 200,
      body: '{"robotId":"robot-1","temperature":22.5}',
    })
    render(<RestView />)
    fireEvent.click(screen.getByRole('button', { name: /Fetch via rest-authz/ }))
    await screen.findByText(/200.*Permit/)
    await screen.findByText(/"robotId"/)
  })

  it('shows 403 Forbidden Deny badge when consumer is not authorized', async () => {
    mockFetchThroughRestAuthz.mockResolvedValue({ status: 403, body: 'Forbidden' })
    render(<RestView />)
    // Switch to an unauthorized consumer
    const selects = screen.getAllByDisplayValue('rest-consumer')
    fireEvent.change(selects[0], { target: { value: 'unknown-system' } })
    fireEvent.click(screen.getByRole('button', { name: /Fetch via rest-authz/ }))
    await screen.findByText(/403.*Deny/)
  })
})

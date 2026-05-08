// Unit tests for GrantsPanel — the grants table + add/revoke controls.
//
// GrantsPanel is the primary UI for managing ConsumerAuthorization grants, which
// drive policy-sync → AuthzForce and are enforced across all three transports.
// Tests here document:
//   - the grants table shape (columns, revoke button per row)
//   - the PROVIDER_MAP contract (which service maps to which provider)
//   - the add-grant flow: disabled when empty, calls addGrant, clears input on success
//   - the revoke flow: calls revokeGrant with the rule's id
//   - error and stale states

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { GrantsPanel } from './GrantsPanel'
import { usePolling } from '../hooks/usePolling'
import { addGrant, revokeGrant } from '../api'

vi.mock('../hooks/usePolling')
vi.mock('../api', () => ({
  fetchAuthRules: vi.fn(),
  addGrant:       vi.fn(),
  revokeGrant:    vi.fn(),
}))

const mockUsePolling  = vi.mocked(usePolling)
const mockAddGrant    = vi.mocked(addGrant)
const mockRevokeGrant = vi.mocked(revokeGrant)

function noData() {
  return { data: null, error: null, loading: false, stale: false, refresh: vi.fn() }
}

const SAMPLE_RULES = [
  { id: 1, consumerSystemName: 'rest-consumer',   providerSystemName: 'data-provider', serviceDefinition: 'telemetry-rest' },
  { id: 2, consumerSystemName: 'consumer-1',      providerSystemName: 'robot-fleet',   serviceDefinition: 'telemetry' },
]

function withRules(rules = SAMPLE_RULES, stale = false) {
  return { data: { rules, count: rules.length }, error: null, loading: false, stale, refresh: vi.fn() }
}

beforeEach(() => {
  mockUsePolling.mockReturnValue(noData())
  mockAddGrant.mockResolvedValue(undefined as never)
  mockRevokeGrant.mockResolvedValue(undefined as never)
})

// ── Static structure ──────────────────────────────────────────────────────────

describe('GrantsPanel — static structure', () => {
  it('renders the "Authorization Grants" heading', () => {
    render(<GrantsPanel />)
    expect(screen.getByText('Authorization Grants')).toBeInTheDocument()
  })

  it('renders the "Add Grant" sub-heading', () => {
    render(<GrantsPanel />)
    expect(screen.getByText('Add Grant')).toBeInTheDocument()
  })

  it('renders the service selector with known service options', () => {
    render(<GrantsPanel />)
    const select = screen.getByRole('combobox')
    const options = Array.from(select.querySelectorAll('option')).map(o => o.textContent)
    expect(options).toContain('telemetry')
    expect(options).toContain('telemetry-rest')
    expect(options).toContain('sensors')
  })
})

// ── Empty and error states ────────────────────────────────────────────────────

describe('GrantsPanel — empty and error states', () => {
  it('shows "No grants." when the rules array is empty', () => {
    mockUsePolling.mockReturnValue(withRules([]))
    render(<GrantsPanel />)
    expect(screen.getByText('No grants.')).toBeInTheDocument()
  })

  it('shows a stale indicator when polling data is stale', () => {
    mockUsePolling.mockReturnValue(withRules(SAMPLE_RULES, /* stale= */ true))
    render(<GrantsPanel />)
    expect(screen.getByText(/stale/)).toBeInTheDocument()
  })
})

// ── Grants table ──────────────────────────────────────────────────────────────

describe('GrantsPanel — grants table', () => {
  it('renders one table row per rule with id, consumer, provider, service columns', () => {
    mockUsePolling.mockReturnValue(withRules())
    render(<GrantsPanel />)
    // Each of these values appears in a <td>
    const tds = Array.from(document.querySelectorAll('td')).map(td => td.textContent)
    expect(tds).toContain('rest-consumer')
    expect(tds).toContain('data-provider')
    expect(tds).toContain('telemetry-rest')
    expect(tds).toContain('consumer-1')
    expect(tds).toContain('robot-fleet')
    expect(tds).toContain('telemetry')
  })

  it('renders a revoke button for each rule', () => {
    mockUsePolling.mockReturnValue(withRules())
    render(<GrantsPanel />)
    const revokeButtons = screen.getAllByRole('button', { name: /revoke/i })
    expect(revokeButtons).toHaveLength(SAMPLE_RULES.length)
  })

  it('calls revokeGrant with the rule id when revoke is clicked', async () => {
    mockUsePolling.mockReturnValue(withRules())
    render(<GrantsPanel />)
    // First revoke button corresponds to rule id=1
    const [firstRevoke] = screen.getAllByRole('button', { name: /revoke/i })
    fireEvent.click(firstRevoke)
    await waitFor(() => expect(mockRevokeGrant).toHaveBeenCalledWith(1))
  })
})

// ── Add grant form ────────────────────────────────────────────────────────────

describe('GrantsPanel — add grant form', () => {
  it('Add button is disabled when the consumer input is empty', () => {
    render(<GrantsPanel />)
    expect(screen.getByRole('button', { name: /^Add$/ })).toBeDisabled()
  })

  it('Add button becomes enabled after typing a consumer name', () => {
    render(<GrantsPanel />)
    fireEvent.change(screen.getByPlaceholderText('consumer system name'), { target: { value: 'new-consumer' } })
    expect(screen.getByRole('button', { name: /^Add$/ })).not.toBeDisabled()
  })

  it('calls addGrant with consumer, correct provider from PROVIDER_MAP, and selected service', async () => {
    render(<GrantsPanel />)
    fireEvent.change(screen.getByPlaceholderText('consumer system name'), { target: { value: 'exp7-consumer' } })
    // Default service is 'telemetry' → provider must be 'robot-fleet' per PROVIDER_MAP
    fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
    await waitFor(() =>
      expect(mockAddGrant).toHaveBeenCalledWith('exp7-consumer', 'robot-fleet', 'telemetry')
    )
  })

  it('maps telemetry-rest service to data-provider', async () => {
    render(<GrantsPanel />)
    fireEvent.change(screen.getByPlaceholderText('consumer system name'), { target: { value: 'c' } })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'telemetry-rest' } })
    fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
    await waitFor(() =>
      expect(mockAddGrant).toHaveBeenCalledWith('c', 'data-provider', 'telemetry-rest')
    )
  })

  it('clears the consumer input after a successful add', async () => {
    render(<GrantsPanel />)
    const input = screen.getByPlaceholderText('consumer system name')
    fireEvent.change(input, { target: { value: 'new-consumer' } })
    fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
    await waitFor(() => expect(input).toHaveValue(''))
  })

  it('shows an error message when addGrant rejects', async () => {
    mockAddGrant.mockRejectedValue(new Error('network error'))
    render(<GrantsPanel />)
    fireEvent.change(screen.getByPlaceholderText('consumer system name'), { target: { value: 'c' } })
    fireEvent.click(screen.getByRole('button', { name: /^Add$/ }))
    await screen.findByText(/network error/)
  })
})

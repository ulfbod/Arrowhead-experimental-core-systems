// Unit tests for ConfigView — dashboard polling intervals and policy-sync control.
//
// ConfigView has two independent concerns:
//   1. SyncIntervalControl: sends POST /config to policy-sync with a new interval.
//      This is experiment-6's key feature — runtime-configurable sync delay.
//   2. Polling config form: local draft of DashboardConfig, applied via setConfig.
//
// Both useConfig and usePolling are mocked.  updateSyncInterval is also mocked so
// tests can assert the correct payload without a live policy-sync service.

import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ConfigView } from './ConfigView'
import { usePolling } from '../hooks/usePolling'
import { useConfig } from '../config/context'
import { updateSyncInterval } from '../api'
import { DEFAULT_CONFIG } from '../config/defaults'

vi.mock('../hooks/usePolling')
vi.mock('../config/context')
vi.mock('../api', () => ({
  fetchPolicySyncStatus: vi.fn(),
  updateSyncInterval:    vi.fn(),
}))

const mockUsePolling         = vi.mocked(usePolling)
const mockUseConfig          = vi.mocked(useConfig)
const mockUpdateSyncInterval = vi.mocked(updateSyncInterval)

function noData() {
  return { data: null, error: null, loading: false, stale: false, refresh: vi.fn() }
}

// ConfigView renders TWO Apply buttons in DOM order:
//   [0] = SyncIntervalControl's Apply (disabled until interval text is entered)
//   [1] = polling-config Apply (always enabled)
// and one "Reset to defaults" button.
function getSyncApply()   { return screen.getAllByRole('button', { name: /^Apply$/ })[0] }
function getConfigApply() { return screen.getAllByRole('button', { name: /^Apply$/ })[1] }

let mockSetConfig:      ReturnType<typeof vi.fn>
let mockResetToDefaults: ReturnType<typeof vi.fn>

beforeEach(() => {
  mockSetConfig       = vi.fn()
  mockResetToDefaults = vi.fn()
  mockUsePolling.mockReturnValue(noData())
  mockUseConfig.mockReturnValue({
    config:          DEFAULT_CONFIG,
    setConfig:       mockSetConfig,
    resetToDefaults: mockResetToDefaults,
  } as ReturnType<typeof useConfig>)
  mockUpdateSyncInterval.mockResolvedValue(undefined as never)
})

// ── Static structure ──────────────────────────────────────────────────────────

describe('ConfigView — static structure', () => {
  it('renders the Policy-Sync Interval section heading', () => {
    render(<ConfigView />)
    expect(screen.getByText('Policy-Sync Interval')).toBeInTheDocument()
  })

  it('renders the Polling intervals section heading', () => {
    render(<ConfigView />)
    expect(screen.getByText('Polling intervals')).toBeInTheDocument()
  })

  it('renders the Display section heading', () => {
    render(<ConfigView />)
    expect(screen.getByText('Display')).toBeInTheDocument()
  })

  it('renders both Apply buttons and a Reset to defaults button', () => {
    render(<ConfigView />)
    expect(screen.getAllByRole('button', { name: /^Apply$/ })).toHaveLength(2)
    expect(screen.getByRole('button', { name: /Reset to defaults/i })).toBeInTheDocument()
  })
})

// ── SyncIntervalControl ───────────────────────────────────────────────────────

describe('ConfigView — SyncIntervalControl', () => {
  it('shows "…" for the current interval when policy-sync status is loading', () => {
    render(<ConfigView />)
    // "Current: " and the interval value are separate DOM nodes — the value is in <strong>
    expect(document.querySelector('strong')?.textContent).toBe('…')
  })

  it('shows the current sync interval from polling data', () => {
    mockUsePolling.mockReturnValue({
      data:    { synced: true, version: 3, lastSyncedAt: '', grants: 2, syncInterval: '15s' },
      error:   null,
      loading: false,
      stale:   false,
      refresh: vi.fn(),
    })
    render(<ConfigView />)
    expect(screen.getByText(/15s/)).toBeInTheDocument()
  })

  it('sync Apply button is disabled while the draft is empty', () => {
    render(<ConfigView />)
    expect(getSyncApply()).toBeDisabled()
  })

  it('sync Apply button becomes enabled after typing an interval', () => {
    render(<ConfigView />)
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. 5s/), { target: { value: '5s' } })
    expect(getSyncApply()).not.toBeDisabled()
  })

  it('calls updateSyncInterval with the typed value on Apply', async () => {
    render(<ConfigView />)
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. 5s/), { target: { value: '5s' } })
    fireEvent.click(getSyncApply())
    await waitFor(() => expect(mockUpdateSyncInterval).toHaveBeenCalledWith('5s'))
  })

  it('shows "✓ applied" after updateSyncInterval resolves', async () => {
    render(<ConfigView />)
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. 5s/), { target: { value: '10s' } })
    fireEvent.click(getSyncApply())
    await screen.findByText(/✓ applied/)
  })

  it('shows an error message when updateSyncInterval rejects', async () => {
    mockUpdateSyncInterval.mockRejectedValue(new Error('policy-sync unreachable'))
    render(<ConfigView />)
    fireEvent.change(screen.getByPlaceholderText(/e\.g\. 5s/), { target: { value: '5s' } })
    fireEvent.click(getSyncApply())
    await screen.findByText(/policy-sync unreachable/)
  })
})

// ── Polling config form ───────────────────────────────────────────────────────

describe('ConfigView — polling config form', () => {
  it('renders the health interval number input', () => {
    render(<ConfigView />)
    // The label "Health check (ms)" wraps a number input via the Field component
    expect(screen.getByText('Health check (ms)')).toBeInTheDocument()
    const inputs = document.querySelectorAll('input[type="number"]')
    expect(inputs.length).toBeGreaterThanOrEqual(1)
  })

  it('initialises number inputs from the current config', () => {
    render(<ConfigView />)
    const inputs = Array.from(document.querySelectorAll('input[type="number"]')) as HTMLInputElement[]
    // At least one input should have the default healthIntervalMs value
    const values = inputs.map(i => Number(i.value))
    expect(values).toContain(DEFAULT_CONFIG.polling.healthIntervalMs)
  })

  it('config Apply button calls setConfig', async () => {
    render(<ConfigView />)
    fireEvent.click(getConfigApply())
    await waitFor(() => expect(mockSetConfig).toHaveBeenCalledTimes(1))
  })

  it('Reset to defaults button calls resetToDefaults', () => {
    render(<ConfigView />)
    fireEvent.click(screen.getByRole('button', { name: /Reset to defaults/i }))
    expect(mockResetToDefaults).toHaveBeenCalledTimes(1)
  })
})

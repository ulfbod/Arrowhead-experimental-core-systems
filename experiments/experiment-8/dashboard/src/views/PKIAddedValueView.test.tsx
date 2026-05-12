// Unit tests for PKIAddedValueView.
//
// Tests cover static rendering (sections present, tables complete, no runtime
// errors) and user interactions (bootstrap button, auth check, service registry
// query). All API calls are mocked; no network or Docker required.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { PKIAddedValueView } from './PKIAddedValueView'

// ── Mock all API calls ────────────────────────────────────────────────────────

vi.mock('../api', () => ({
  fetchCAInfo:            vi.fn(),
  issueOnboardingCert:    vi.fn(),
  fetchPKIConsumerStats:  vi.fn(),
  fetchPKIRestAuthzStatus:vi.fn(),
  fetchAuthRules:         vi.fn(),
  checkPKIRestAuthz:      vi.fn(),
  queryServiceRegistry:   vi.fn(),
}))

// usePolling returns data immediately via a synchronous mock.
vi.mock('../hooks/usePolling', () => ({
  usePolling: vi.fn((fetcher: (s?: AbortSignal) => Promise<unknown>) => {
    // Return a minimal object so component renders without crashing.
    return { data: null, error: null, stale: false, loading: false, refresh: vi.fn() }
  }),
}))

import * as api from '../api'
import { usePolling } from '../hooks/usePolling'

// ── Helpers ───────────────────────────────────────────────────────────────────

function renderView() {
  return render(<PKIAddedValueView />)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('PKIAddedValueView', () => {

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders all seven section headings', () => {
    renderView()
    // Use heading role queries to avoid matching content in paragraphs/diagrams
    const headings = screen.getAllByRole('heading')
    const headingTexts = headings.map(h => h.textContent ?? '')
    expect(headingTexts.some(t => /PKI Lifecycle Walkthrough/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Interactive Bootstrap/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Profile Enforcement.*Allowed/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Access-Policy Lifecycle/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Identity-to-Authorization Trace/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Service Registry Lookup/i.test(t))).toBe(true)
    expect(headingTexts.some(t => /Feature Comparison/i.test(t))).toBe(true)
  })

  it('renders the lifecycle diagram', () => {
    renderView()
    expect(screen.getByText(/on→de→sy/i)).toBeInTheDocument()
  })

  it('renders the evidence banner with placeholder values when data is null', () => {
    renderView()
    // Multiple '…' placeholders expected (messages received, permitted, denied, last message)
    const placeholders = screen.getAllByText('…')
    expect(placeholders.length).toBeGreaterThanOrEqual(4)
  })

  it('renders the live stats banner with real data when usePolling returns data', () => {
    // Cycle through 4 usePolling calls: caInfo, pkiStats, pkiAuthzStatus, grants
    let callCount = 0
    vi.mocked(usePolling).mockImplementation(() => {
      const idx = callCount++
      if (idx === 1) { // pkiStats
        return { data: { msgCount: 42, lastReceivedAt: '2024-01-01T12:00:00Z', transport: 'rest-mtls-pki', deniedCount: 2, lastDeniedAt: '' }, error: null, stale: false, loading: false, refresh: vi.fn() }
      }
      if (idx === 2) { // pkiAuthzStatus
        return { data: { requestsTotal: 50, permitted: 48, denied: 2 }, error: null, stale: false, loading: false, refresh: vi.fn() }
      }
      return { data: null, error: null, stale: false, loading: false, refresh: vi.fn() }
    })
    renderView()
    // Evidence banner shows real values (multiple elements may contain these numbers)
    expect(screen.getAllByText('42').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('48').length).toBeGreaterThanOrEqual(1)
  })

  it('bootstrap button calls issueOnboardingCert with the system name', async () => {
    vi.mocked(api.issueOnboardingCert).mockResolvedValue({
      certificate: '-----BEGIN CERTIFICATE-----\nMOCK\n-----END CERTIFICATE-----',
      privateKey:  '-----BEGIN EC PRIVATE KEY-----\nMOCK\n-----END EC PRIVATE KEY-----',
      profile:     'on',
    })

    renderView()

    const btn = screen.getByRole('button', { name: /Bootstrap.*OU=on/i })
    fireEvent.click(btn)

    await waitFor(() => {
      expect(api.issueOnboardingCert).toHaveBeenCalledWith('demo-system')
    })
  })

  it('shows cert result after successful bootstrap', async () => {
    vi.mocked(api.issueOnboardingCert).mockResolvedValue({
      certificate: '-----BEGIN CERTIFICATE-----\nFAKEDATA\n-----END CERTIFICATE-----',
      privateKey:  '-----BEGIN EC PRIVATE KEY-----\nFAKEKEY\n-----END EC PRIVATE KEY-----',
      profile:     'on',
    })

    renderView()
    fireEvent.click(screen.getByRole('button', { name: /Bootstrap.*OU=on/i }))

    await waitFor(() => {
      expect(screen.getByText(/on — onboarding/i)).toBeInTheDocument()
    })
    expect(screen.getByText(/step 1 \/ 4 complete/i)).toBeInTheDocument()
  })

  it('shows error message when bootstrap fails', async () => {
    vi.mocked(api.issueOnboardingCert).mockRejectedValue(new Error('HTTP 500: internal error'))

    renderView()
    fireEvent.click(screen.getByRole('button', { name: /Bootstrap.*OU=on/i }))

    await waitFor(() => {
      expect(screen.getByText(/HTTP 500: internal error/i)).toBeInTheDocument()
    })
  })

  it('check authz button calls checkPKIRestAuthz', async () => {
    vi.mocked(api.checkPKIRestAuthz).mockResolvedValue({
      decision: 'Permit',
      permit:   true,
      consumer: 'pki-consumer',
      service:  'telemetry-rest',
    })

    renderView()
    const checkBtn = screen.getByRole('button', { name: /Check AuthzForce/i })
    fireEvent.click(checkBtn)

    await waitFor(() => {
      expect(api.checkPKIRestAuthz).toHaveBeenCalledWith('pki-consumer', 'telemetry-rest')
    })
  })

  it('shows Permit decision result', async () => {
    vi.mocked(api.checkPKIRestAuthz).mockResolvedValue({
      decision: 'Permit',
      permit:   true,
      consumer: 'pki-consumer',
      service:  'telemetry-rest',
    })

    renderView()
    fireEvent.click(screen.getByRole('button', { name: /Check AuthzForce/i }))

    await waitFor(() => {
      expect(screen.getByText('Permit')).toBeInTheDocument()
    })
    expect(screen.getByText(/Access permitted/i)).toBeInTheDocument()
  })

  it('shows Deny decision result', async () => {
    vi.mocked(api.checkPKIRestAuthz).mockResolvedValue({
      decision: 'Deny',
      permit:   false,
      consumer: 'unauthorized',
      service:  'telemetry-rest',
    })

    renderView()
    fireEvent.click(screen.getByRole('button', { name: /Check AuthzForce/i }))

    await waitFor(() => {
      expect(screen.getByText('Deny')).toBeInTheDocument()
    })
    expect(screen.getByText(/Access denied/i)).toBeInTheDocument()
  })

  it('service registry query button calls queryServiceRegistry', async () => {
    vi.mocked(api.queryServiceRegistry).mockResolvedValue({
      serviceQueryData: [],
    })

    renderView()
    const srBtn = screen.getByRole('button', { name: /Query ServiceRegistry/i })
    fireEvent.click(srBtn)

    await waitFor(() => {
      expect(api.queryServiceRegistry).toHaveBeenCalledWith('telemetry-rest')
    })
  })

  it('shows service registry results', async () => {
    vi.mocked(api.queryServiceRegistry).mockResolvedValue({
      serviceQueryData: [
        {
          id: 1,
          serviceDefinition: 'telemetry-rest',
          interfaces: ['HTTPS-SECURE-JSON'],
          providerSystem: { systemName: 'data-provider-tls', address: 'localhost', port: 9094 },
          serviceUri: '',
          version: 1,
        },
      ],
      unfilteredHits: 1,
    })

    renderView()
    fireEvent.click(screen.getByRole('button', { name: /Query ServiceRegistry/i }))

    await waitFor(() => {
      expect(screen.getByText('data-provider-tls')).toBeInTheDocument()
    })
    expect(screen.getByText('1 provider(s) found')).toBeInTheDocument()
  })

  it('profile enforcement table has all expected rows', () => {
    renderView()
    // OU columns (multiple rows expected for OU=sy cert)
    expect(screen.getAllByText('OU=sy cert').length).toBeGreaterThan(0)
    expect(screen.getAllByText(/OU=on cert/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/OU=de cert/).length).toBeGreaterThan(0)
    // All endpoints represented
    expect(screen.getAllByText('/profile/device-cert :8088').length).toBeGreaterThan(0)
    expect(screen.getAllByText('pki-rest-authz mTLS :9108').length).toBeGreaterThan(0)
  })

  it('extensions comparison table has key rows', () => {
    renderView()
    expect(screen.getByText('Certificate hierarchy')).toBeInTheDocument()
    expect(screen.getByText('Certificate profiles')).toBeInTheDocument()
    expect(screen.getByText('Profile enforcement at PEP')).toBeInTheDocument()
    expect(screen.getByText('Bootstrap endpoint')).toBeInTheDocument()
  })

  it('steps 2-4 explanations are visible', () => {
    renderView()
    expect(screen.getByText('Step 2')).toBeInTheDocument()
    expect(screen.getByText('Step 3')).toBeInTheDocument()
    expect(screen.getByText('Step 4')).toBeInTheDocument()
  })

})

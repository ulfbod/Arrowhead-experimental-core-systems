// Unit tests for the new API functions added alongside the Kafka and REST tabs.

import { describe, it, expect, vi, afterEach } from 'vitest'
import {
  checkKafkaAuthz,
  fetchDataProviderStats,
  queryServiceRegistry,
  fetchThroughRestAuthz,
} from './api'

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Stubs globalThis.fetch to return a controlled response. */
function stubFetch(status: number, body: unknown): void {
  const isJson = typeof body !== 'string'
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
    ok:   status >= 200 && status < 300,
    status,
    json: isJson ? () => Promise.resolve(body) : () => Promise.reject(new Error('not json')),
    text: () => Promise.resolve(isJson ? JSON.stringify(body) : body as string),
  }))
}

afterEach(() => vi.unstubAllGlobals())

// ── checkKafkaAuthz ───────────────────────────────────────────────────────────

describe('checkKafkaAuthz', () => {
  it('POSTs to /api/kafka-authz/auth/check with consumer and service', async () => {
    stubFetch(200, { consumer: 'analytics-consumer', service: 'telemetry', permit: true, decision: 'Permit' })

    await checkKafkaAuthz('analytics-consumer', 'telemetry')

    const [url, init] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/kafka-authz/auth/check')
    expect(init.method).toBe('POST')
    expect(init.headers['Content-Type']).toBe('application/json')
    expect(JSON.parse(init.body)).toEqual({ consumer: 'analytics-consumer', service: 'telemetry' })
  })

  it('returns parsed Permit result', async () => {
    stubFetch(200, { consumer: 'c', service: 's', permit: true, decision: 'Permit' })
    const result = await checkKafkaAuthz('c', 's')
    expect(result.permit).toBe(true)
    expect(result.decision).toBe('Permit')
  })

  it('returns parsed Deny result', async () => {
    stubFetch(200, { consumer: 'c', service: 's', permit: false, decision: 'Deny' })
    const result = await checkKafkaAuthz('c', 's')
    expect(result.permit).toBe(false)
    expect(result.decision).toBe('Deny')
  })

  it('throws with HTTP status on non-OK response', async () => {
    stubFetch(503, 'PDP unavailable')
    await expect(checkKafkaAuthz('x', 'y')).rejects.toThrow('HTTP 503')
  })
})

// ── fetchDataProviderStats ────────────────────────────────────────────────────

describe('fetchDataProviderStats', () => {
  it('GETs /api/data-provider/stats', async () => {
    stubFetch(200, { msgCount: 42, robotCount: 3, lastReceivedAt: '2026-01-01T00:00:00Z' })
    await fetchDataProviderStats()
    expect((fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe('/api/data-provider/stats')
  })

  it('returns parsed stats', async () => {
    stubFetch(200, { msgCount: 100, robotCount: 2, lastReceivedAt: '2026-05-01T12:00:00Z' })
    const result = await fetchDataProviderStats()
    expect(result.msgCount).toBe(100)
    expect(result.robotCount).toBe(2)
    expect(result.lastReceivedAt).toBe('2026-05-01T12:00:00Z')
  })
})

// ── queryServiceRegistry ──────────────────────────────────────────────────────

describe('queryServiceRegistry', () => {
  it('POSTs to /api/serviceregistry/serviceregistry/query', async () => {
    stubFetch(200, { serviceInstances: [], count: 0 })
    await queryServiceRegistry('telemetry-rest')
    const [url, init] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/serviceregistry/serviceregistry/query')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ serviceDefinition: 'telemetry-rest' })
  })

  it('returns empty result when no instances registered', async () => {
    stubFetch(200, { serviceInstances: [], count: 0 })
    const result = await queryServiceRegistry('telemetry-rest')
    expect(result.serviceInstances).toHaveLength(0)
    expect(result.count).toBe(0)
  })

  it('returns registered service instances', async () => {
    const instance = {
      id: 1,
      serviceDefinition: 'telemetry-rest',
      providerSystem: { systemName: 'data-provider', address: 'data-provider', port: 9094 },
      serviceUri: '/telemetry/latest',
      interfaces: ['HTTP-INSECURE-JSON'],
      version: 1,
    }
    stubFetch(200, { serviceInstances: [instance], count: 1 })
    const result = await queryServiceRegistry('telemetry-rest')
    expect(result.count).toBe(1)
    expect(result.serviceInstances[0].providerSystem.systemName).toBe('data-provider')
    expect(result.serviceInstances[0].serviceUri).toBe('/telemetry/latest')
  })

  it('throws on non-OK response', async () => {
    stubFetch(500, 'internal error')
    await expect(queryServiceRegistry('foo')).rejects.toThrow('HTTP 500')
  })
})

// ── fetchThroughRestAuthz ─────────────────────────────────────────────────────

describe('fetchThroughRestAuthz', () => {
  it('GETs /api/rest-authz/telemetry/latest with X-Consumer-Name header', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      status: 200,
      text: () => Promise.resolve('{"robotId":"robot-1"}'),
    }))
    await fetchThroughRestAuthz('rest-consumer')
    const [url, init] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/rest-authz/telemetry/latest')
    expect(init.headers['X-Consumer-Name']).toBe('rest-consumer')
  })

  it('returns status 200 and body for an authorized consumer', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      status: 200,
      text: () => Promise.resolve('{"robotId":"robot-1","temperature":23.5}'),
    }))
    const result = await fetchThroughRestAuthz('rest-consumer')
    expect(result.status).toBe(200)
    expect(result.body).toContain('"robotId"')
  })

  it('returns status 403 and body for an unauthorized consumer', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      status: 403,
      text: () => Promise.resolve('Forbidden'),
    }))
    const result = await fetchThroughRestAuthz('unauthorized')
    expect(result.status).toBe(403)
    expect(result.body).toBe('Forbidden')
  })

  it('passes the consumer name from the argument', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      status: 200,
      text: () => Promise.resolve('{}'),
    }))
    await fetchThroughRestAuthz('test-probe')
    const init = (fetch as ReturnType<typeof vi.fn>).mock.calls[0][1]
    expect(init.headers['X-Consumer-Name']).toBe('test-probe')
  })
})

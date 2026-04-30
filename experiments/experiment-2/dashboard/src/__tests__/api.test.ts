import { vi, describe, it, expect, beforeEach } from 'vitest'
import {
  fetchHealth,
  fetchAllServices,
  fetchAuthRules,
  fetchQueues,
  fetchExchanges,
  fetchTelemetry,
  fetchOrchestration,
  fetchFleetConfig,
  postFleetConfig,
  fetchTelemetryStats,
  fetchAllTelemetry,
} from '../api'
import type { FleetConfig } from '../types'

// Replace global fetch with a controllable mock.
function mockFetch(status: number, body: unknown) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  })
}

describe('fetchHealth', () => {
  it('returns health response', async () => {
    globalThis.fetch = mockFetch(200, { status: 'ok', system: 'ca' })
    const data = await fetchHealth('/api/ca')
    expect(data.status).toBe('ok')
  })

  it('throws on non-ok response', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false, status: 503,
      text: () => Promise.resolve('down'),
      json: () => Promise.resolve({ error: 'down' }),
    })
    await expect(fetchHealth('/api/ca')).rejects.toThrow('HTTP 503')
  })
})

describe('fetchAllServices', () => {
  beforeEach(() => {
    globalThis.fetch = mockFetch(200, {
      serviceQueryData: [
        { id: 1, serviceDefinition: 'telemetry', providerSystem: { systemName: 'edge-adapter', address: 'localhost', port: 9001 }, serviceUri: '/telemetry/latest', interfaces: ['HTTP-INSECURE-JSON'], version: 1 },
      ],
      unfilteredHits: 1,
    })
  })

  it('returns services array', async () => {
    const data = await fetchAllServices()
    expect(data.serviceQueryData).toHaveLength(1)
    expect(data.serviceQueryData[0].serviceDefinition).toBe('telemetry')
  })

  it('sends POST to correct path', async () => {
    await fetchAllServices()
    expect((globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0]).toBe(
      '/api/sr/serviceregistry/query',
    )
  })
})

describe('fetchAuthRules', () => {
  it('returns rules array', async () => {
    globalThis.fetch = mockFetch(200, {
      rules: [{ id: 1, consumerSystemName: 'demo-consumer', providerSystemName: 'edge-adapter', serviceDefinition: 'telemetry' }],
      count: 1,
    })
    const data = await fetchAuthRules()
    expect(data.rules).toHaveLength(1)
  })
})

describe('fetchQueues', () => {
  it('sends Authorization header with Basic auth', async () => {
    globalThis.fetch = mockFetch(200, [])
    await fetchQueues()
    const init = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>)['Authorization']).toMatch(/^Basic /)
  })
})

describe('fetchExchanges', () => {
  it('returns exchange list', async () => {
    globalThis.fetch = mockFetch(200, [{ name: 'arrowhead', type: 'topic', durable: true }])
    const data = await fetchExchanges()
    expect(data[0].name).toBe('arrowhead')
  })
})

describe('fetchTelemetry', () => {
  it('returns null on 204 No Content', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, status: 204 })
    const data = await fetchTelemetry()
    expect(data).toBeNull()
  })

  it('returns telemetry payload on 200', async () => {
    const payload = { receivedAt: '2026-01-01T00:00:00Z', payload: { robotId: 'robot-1', temperature: 22, humidity: 55, timestamp: '', seq: 1 } }
    globalThis.fetch = mockFetch(200, payload)
    const data = await fetchTelemetry()
    expect(data?.payload.robotId).toBe('robot-1')
  })

  it('throws on non-ok, non-204 response', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 502 })
    await expect(fetchTelemetry()).rejects.toThrow('HTTP 502')
  })
})

describe('fetchFleetConfig', () => {
  it('fetches from /api/robot-fleet/config', async () => {
    const cfg: FleetConfig = { payloadType: 'imu', payloadHz: 10, robots: [] }
    globalThis.fetch = mockFetch(200, cfg)
    const data = await fetchFleetConfig()
    expect(data.payloadType).toBe('imu')
    const url = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0][0] as string
    expect(url).toContain('/api/robot-fleet/config')
  })
})

describe('postFleetConfig', () => {
  it('sends PUT/POST to /api/robot-fleet/config', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, status: 204, text: () => Promise.resolve('') })
    const cfg: FleetConfig = { payloadType: 'basic', payloadHz: 5, robots: [{ id: 'r1', networkPreset: '5g_good' }] }
    await postFleetConfig(cfg)
    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(call[0]).toContain('/api/robot-fleet/config')
    expect(call[1].method).toBe('POST')
  })

  it('throws on non-ok response', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 400, text: () => Promise.resolve('bad') })
    await expect(postFleetConfig({ payloadType: 'imu', payloadHz: 10, robots: [] })).rejects.toThrow('HTTP 400')
  })
})

describe('fetchTelemetryStats', () => {
  it('fetches from /api/telemetry/telemetry/stats', async () => {
    const resp = { robots: {}, aggregate: { robotCount: 0, totalMsgCount: 0, meanLatencyMs: 0, p95LatencyMs: 0 } }
    globalThis.fetch = mockFetch(200, resp)
    const data = await fetchTelemetryStats()
    expect(data.aggregate.robotCount).toBe(0)
  })
})

describe('fetchAllTelemetry', () => {
  it('fetches from /api/telemetry/telemetry/all', async () => {
    globalThis.fetch = mockFetch(200, {})
    const data = await fetchAllTelemetry()
    expect(typeof data).toBe('object')
  })
})

describe('fetchOrchestration', () => {
  it('sends POST with given consumer name', async () => {
    globalThis.fetch = mockFetch(200, { response: [] })
    await fetchOrchestration('demo-consumer', 'telemetry')
    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0]
    const body = JSON.parse(call[1].body as string) as { requesterSystem: { systemName: string }; requestedService: { serviceDefinition: string } }
    expect(body.requesterSystem.systemName).toBe('demo-consumer')
    expect(body.requestedService.serviceDefinition).toBe('telemetry')
  })

  it('returns empty response array when no providers', async () => {
    globalThis.fetch = mockFetch(200, { response: [] })
    const data = await fetchOrchestration('demo-consumer', 'telemetry')
    expect(data.response).toHaveLength(0)
  })
})

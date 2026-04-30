import type { DashboardConfig } from './types'

export const DEFAULT_CONFIG: DashboardConfig = {
  polling: {
    healthIntervalMs:        10_000,
    servicesIntervalMs:      10_000,
    telemetryIntervalMs:      2_000,
    orchIntervalMs:          10_000,
    brokerIntervalMs:         5_000,
    fleetStatsIntervalMs:     3_000,
    allTelemetryIntervalMs:   3_000,
  },
  display: {
    maxTelemetryHistory: 20,
    showHealthLatency:   true,
  },
  experiment2: {
    consumerName:       'demo-consumer',
    serviceDefinition:  'telemetry',
    pollIntervalLabel:  '5s (set in docker-compose)',
  },
}

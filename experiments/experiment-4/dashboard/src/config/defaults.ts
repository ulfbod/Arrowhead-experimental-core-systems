import type { DashboardConfig } from './types'

export const DEFAULT_CONFIG: DashboardConfig = {
  polling: {
    healthIntervalMs:        10_000,
    grantsIntervalMs:         5_000,
    rmqUsersIntervalMs:       5_000,
    consumerStatsIntervalMs:  3_000,
  },
  display: {
    showHealthLatency: true,
  },
}

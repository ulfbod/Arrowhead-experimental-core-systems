export interface PollingConfig {
  healthIntervalMs:        number
  grantsIntervalMs:        number
  rmqUsersIntervalMs:      number
  consumerStatsIntervalMs: number
}

export interface DisplayConfig {
  showHealthLatency: boolean
}

export interface DashboardConfig {
  polling: PollingConfig
  display: DisplayConfig
}

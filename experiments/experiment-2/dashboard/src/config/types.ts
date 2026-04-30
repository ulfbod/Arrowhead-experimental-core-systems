// Dashboard configuration types.
// The config is separated into three namespaces so future experiments or
// support systems can add their own sections without touching the shared ones.

export interface PollingConfig {
  healthIntervalMs: number       // health endpoint poll rate
  servicesIntervalMs: number     // service-registry query rate
  telemetryIntervalMs: number    // edge-adapter telemetry poll rate
  orchIntervalMs: number         // orchestration query rate
  brokerIntervalMs: number       // RabbitMQ stats poll rate
}

export interface DisplayConfig {
  maxTelemetryHistory: number    // rolling telemetry row count
  showHealthLatency: boolean     // show response-time in health grid
}

export interface Experiment2Config {
  consumerName: string           // requesterSystem.systemName for orch queries
  serviceDefinition: string      // requestedService.serviceDefinition
  pollIntervalLabel: string      // human-readable label only (informational)
}

export interface DashboardConfig {
  polling:     PollingConfig
  display:     DisplayConfig
  experiment2: Experiment2Config
}

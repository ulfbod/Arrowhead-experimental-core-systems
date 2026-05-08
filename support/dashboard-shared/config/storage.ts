// Stub storage implementation used when testing support/dashboard-shared in isolation.
// When context.tsx is symlinked into an experiment's dashboard/src/config/, the
// experiment's own storage.ts is used instead of this file.
import type { DashboardConfig } from './types'
import { DEFAULT_CONFIG } from './defaults'

export function loadConfig(): DashboardConfig {
  return { ...DEFAULT_CONFIG }
}

export function saveConfig(_cfg: DashboardConfig): void {}

export function resetConfig(): DashboardConfig {
  return { ...DEFAULT_CONFIG }
}

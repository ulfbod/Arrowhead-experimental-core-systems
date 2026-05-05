import type { DashboardConfig } from './types'
import { DEFAULT_CONFIG } from './defaults'

const KEY = 'arrowhead-exp6-config'

function merge(defaults: DashboardConfig, saved: Partial<DashboardConfig>): DashboardConfig {
  return {
    polling: { ...defaults.polling, ...(saved.polling ?? {}) },
    display: { ...defaults.display, ...(saved.display ?? {}) },
  }
}

export function loadConfig(): DashboardConfig {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return DEFAULT_CONFIG
    return merge(DEFAULT_CONFIG, JSON.parse(raw) as Partial<DashboardConfig>)
  } catch {
    return DEFAULT_CONFIG
  }
}

export function saveConfig(cfg: DashboardConfig): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(cfg))
  } catch { /* ignore */ }
}

export function resetConfig(): DashboardConfig {
  try { localStorage.removeItem(KEY) } catch { /* ignore */ }
  return DEFAULT_CONFIG
}

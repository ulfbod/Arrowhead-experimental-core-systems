import { describe, it, expect, beforeEach } from 'vitest'
import { loadConfig, saveConfig, resetConfig } from '../config/storage'
import { DEFAULT_CONFIG } from '../config/defaults'

// jsdom provides a localStorage stub; reset it between tests.
beforeEach(() => localStorage.clear())

describe('loadConfig', () => {
  it('returns DEFAULT_CONFIG when localStorage is empty', () => {
    expect(loadConfig()).toEqual(DEFAULT_CONFIG)
  })

  it('deep-merges saved polling values over defaults', () => {
    localStorage.setItem('arrowhead-exp2-config', JSON.stringify({
      polling: { healthIntervalMs: 999 },
    }))
    const cfg = loadConfig()
    expect(cfg.polling.healthIntervalMs).toBe(999)
    // Other polling keys still have defaults.
    expect(cfg.polling.telemetryIntervalMs).toBe(DEFAULT_CONFIG.polling.telemetryIntervalMs)
  })

  it('returns defaults if localStorage contains invalid JSON', () => {
    localStorage.setItem('arrowhead-exp2-config', 'not-json')
    expect(loadConfig()).toEqual(DEFAULT_CONFIG)
  })
})

describe('saveConfig / loadConfig round-trip', () => {
  it('persists a modified config and reads it back', () => {
    const modified = {
      ...DEFAULT_CONFIG,
      polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 5000 },
    }
    saveConfig(modified)
    const loaded = loadConfig()
    expect(loaded.polling.healthIntervalMs).toBe(5000)
  })
})

describe('resetConfig', () => {
  it('clears localStorage and returns defaults', () => {
    saveConfig({ ...DEFAULT_CONFIG, polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 1 } })
    const cfg = resetConfig()
    expect(cfg).toEqual(DEFAULT_CONFIG)
    expect(loadConfig()).toEqual(DEFAULT_CONFIG)
  })
})

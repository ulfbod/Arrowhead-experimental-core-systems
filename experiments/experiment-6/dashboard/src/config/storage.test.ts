// Unit tests for storage.ts — the experiment-6-specific localStorage layer.
//
// storage.ts is NOT shared (not symlinked from dashboard-shared); it holds the
// localStorage key 'arrowhead-exp6-config' and a merge helper that shallow-merges
// saved sub-objects over DEFAULT_CONFIG.  These tests document that contract so
// an AI templating storage.ts for experiment-7 knows exactly what to preserve.

import { describe, it, expect, beforeEach } from 'vitest'
import { loadConfig, saveConfig, resetConfig } from './storage'
import { DEFAULT_CONFIG } from './defaults'

const KEY = 'arrowhead-exp6-config'

describe('storage', () => {
  beforeEach(() => localStorage.clear())

  // ── loadConfig ──────────────────────────────────────────────────────────────

  describe('loadConfig', () => {
    it('returns DEFAULT_CONFIG when localStorage is empty', () => {
      expect(loadConfig()).toEqual(DEFAULT_CONFIG)
    })

    it('merges a saved polling sub-object over defaults, leaving other fields intact', () => {
      localStorage.setItem(KEY, JSON.stringify({ polling: { healthIntervalMs: 9999 } }))
      const cfg = loadConfig()
      expect(cfg.polling.healthIntervalMs).toBe(9999)
      // Fields not present in the saved object come from DEFAULT_CONFIG
      expect(cfg.polling.grantsIntervalMs).toBe(DEFAULT_CONFIG.polling.grantsIntervalMs)
      expect(cfg.polling.policyIntervalMs).toBe(DEFAULT_CONFIG.polling.policyIntervalMs)
    })

    it('merges a saved display sub-object over defaults', () => {
      const flipped = !DEFAULT_CONFIG.display.showHealthLatency
      localStorage.setItem(KEY, JSON.stringify({ display: { showHealthLatency: flipped } }))
      const cfg = loadConfig()
      expect(cfg.display.showHealthLatency).toBe(flipped)
    })

    it('returns DEFAULT_CONFIG when localStorage contains invalid JSON', () => {
      localStorage.setItem(KEY, '{bad json}')
      expect(loadConfig()).toEqual(DEFAULT_CONFIG)
    })

    it('returns DEFAULT_CONFIG when the saved value is missing the polling key', () => {
      // Only display saved — polling should fall back to defaults
      localStorage.setItem(KEY, JSON.stringify({ display: {} }))
      const cfg = loadConfig()
      expect(cfg.polling).toEqual(DEFAULT_CONFIG.polling)
    })
  })

  // ── saveConfig ──────────────────────────────────────────────────────────────

  describe('saveConfig', () => {
    it('persists values so that loadConfig returns them', () => {
      const custom = {
        ...DEFAULT_CONFIG,
        polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 1234 },
      }
      saveConfig(custom)
      expect(loadConfig().polling.healthIntervalMs).toBe(1234)
    })

    it('writes a JSON string under the experiment key', () => {
      saveConfig(DEFAULT_CONFIG)
      const raw = localStorage.getItem(KEY)
      expect(raw).not.toBeNull()
      expect(() => JSON.parse(raw!)).not.toThrow()
    })

    it('overwrites a previously saved value', () => {
      saveConfig({ ...DEFAULT_CONFIG, polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 100 } })
      saveConfig({ ...DEFAULT_CONFIG, polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 200 } })
      expect(loadConfig().polling.healthIntervalMs).toBe(200)
    })
  })

  // ── resetConfig ─────────────────────────────────────────────────────────────

  describe('resetConfig', () => {
    it('returns DEFAULT_CONFIG', () => {
      expect(resetConfig()).toEqual(DEFAULT_CONFIG)
    })

    it('removes saved values so that subsequent loadConfig returns defaults', () => {
      saveConfig({ ...DEFAULT_CONFIG, polling: { ...DEFAULT_CONFIG.polling, healthIntervalMs: 7777 } })
      resetConfig()
      expect(loadConfig().polling.healthIntervalMs).toBe(DEFAULT_CONFIG.polling.healthIntervalMs)
    })

    it('is idempotent — calling twice does not throw', () => {
      expect(() => { resetConfig(); resetConfig() }).not.toThrow()
    })
  })
})

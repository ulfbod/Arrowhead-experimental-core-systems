// Tests for the DEFAULT_CONFIG constant.

import { describe, it, expect } from 'vitest'
import { DEFAULT_CONFIG } from './defaults'

describe('DEFAULT_CONFIG — shape', () => {
  it('has a polling section', () => {
    expect(DEFAULT_CONFIG.polling).toBeDefined()
  })

  it('has a display section', () => {
    expect(DEFAULT_CONFIG.display).toBeDefined()
  })
})

describe('DEFAULT_CONFIG — polling intervals', () => {
  const intervalKeys = [
    'healthIntervalMs',
    'grantsIntervalMs',
    'rmqUsersIntervalMs',
    'consumerStatsIntervalMs',
    'policyIntervalMs',
  ] as const

  for (const key of intervalKeys) {
    it(`${key} is a positive integer (milliseconds)`, () => {
      const val = DEFAULT_CONFIG.polling[key]
      expect(val).toBeTypeOf('number')
      expect(Number.isInteger(val)).toBe(true)
      expect(val).toBeGreaterThan(0)
    })
  }
})

describe('DEFAULT_CONFIG — display', () => {
  it('showHealthLatency is a boolean', () => {
    expect(DEFAULT_CONFIG.display.showHealthLatency).toBeTypeOf('boolean')
  })
})

// Tests for the usePolling hook.
//
// Covers: initial state, successful fetch, error handling, stale-data flag,
// poll interval, cleanup on unmount, and the refresh() function.
//
// NOTE: vi.runAllTimersAsync() must NOT be used here — the hook's setInterval
// never stops, causing vitest to abort after 10 000 timer iterations.
// Use flush() and tick() instead.

import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { usePolling } from './usePolling'

beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers() })

// Drain the microtask queue so that in-flight promise .then/.catch callbacks run.
const flush = () =>
  act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })

// Advance fake timers by ms (fires any setInterval callbacks synchronously),
// then drain the microtask queue.
const tick = (ms: number) =>
  act(async () => {
    vi.advanceTimersByTime(ms)
    await Promise.resolve()
    await Promise.resolve()
  })

// ── Initial state ─────────────────────────────────────────────────────────────

describe('usePolling — initial state', () => {
  it('starts with loading=true, data=null, error=null, stale=false', () => {
    const fetcher = vi.fn().mockReturnValue(new Promise(() => {}))
    const { result } = renderHook(() => usePolling(fetcher, 5000))
    expect(result.current).toMatchObject({
      loading: true,
      data:    null,
      error:   null,
      stale:   false,
    })
  })

  it('calls the fetcher immediately on mount', () => {
    const fetcher = vi.fn().mockReturnValue(new Promise(() => {}))
    renderHook(() => usePolling(fetcher, 5000))
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('passes an AbortSignal to the fetcher', () => {
    const fetcher = vi.fn().mockReturnValue(new Promise(() => {}))
    renderHook(() => usePolling(fetcher, 5000))
    expect(fetcher).toHaveBeenCalledWith(expect.any(AbortSignal))
  })
})

// ── Successful fetch ──────────────────────────────────────────────────────────

describe('usePolling — successful fetch', () => {
  it('sets data and clears loading after a successful fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue({ count: 3 })
    const { result } = renderHook(() => usePolling(fetcher, 5000))
    await flush()
    expect(result.current).toMatchObject({
      loading: false,
      data:    { count: 3 },
      error:   null,
      stale:   false,
    })
  })

  it('updates data when a later fetch returns a new value', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce({ n: 1 })
      .mockResolvedValue({ n: 2 })
    const { result } = renderHook(() => usePolling(fetcher, 1000))
    await flush()
    expect(result.current.data).toEqual({ n: 1 })

    await tick(1000)
    expect(result.current.data).toEqual({ n: 2 })
    expect(result.current.error).toBeNull()
    expect(result.current.stale).toBe(false)
  })
})

// ── Error handling ────────────────────────────────────────────────────────────

describe('usePolling — error handling', () => {
  it('sets error and clears loading when the fetcher rejects', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('net error'))
    const { result } = renderHook(() => usePolling(fetcher, 5000))
    await flush()
    expect(result.current.error).toBe('net error')
    expect(result.current.loading).toBe(false)
    expect(result.current.data).toBeNull()
  })

  it('stale=false when an error arrives with no prior data', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('fail'))
    const { result } = renderHook(() => usePolling(fetcher, 5000))
    await flush()
    expect(result.current.stale).toBe(false)
  })

  it('stale=true and preserves data when an error follows a successful fetch', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce({ n: 7 })
      .mockRejectedValue(new Error('transient'))
    const { result } = renderHook(() => usePolling(fetcher, 1000))

    await flush()
    expect(result.current.data).toEqual({ n: 7 })

    await tick(1000)
    expect(result.current.stale).toBe(true)
    expect(result.current.data).toEqual({ n: 7 })
    expect(result.current.error).toBe('transient')
  })

  it('converts non-Error rejections to a string message', async () => {
    const fetcher = vi.fn().mockRejectedValue('plain string error')
    const { result } = renderHook(() => usePolling(fetcher, 5000))
    await flush()
    expect(result.current.error).toBe('plain string error')
  })
})

// ── Polling interval ──────────────────────────────────────────────────────────

describe('usePolling — polling interval', () => {
  it('fetches again after the interval elapses', async () => {
    const fetcher = vi.fn().mockResolvedValue(null)
    renderHook(() => usePolling(fetcher, 1000))
    await flush()
    expect(fetcher).toHaveBeenCalledTimes(1)

    await tick(1000)
    expect(fetcher).toHaveBeenCalledTimes(2)

    await tick(1000)
    expect(fetcher).toHaveBeenCalledTimes(3)
  })

  it('stops fetching after the component unmounts', async () => {
    const fetcher = vi.fn().mockResolvedValue(null)
    const { unmount } = renderHook(() => usePolling(fetcher, 1000))
    await flush()
    unmount()

    await tick(5000)
    // Still only the one call made before unmount
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
})

// ── refresh() ─────────────────────────────────────────────────────────────────

describe('usePolling — refresh()', () => {
  it('triggers an immediate re-fetch before the next interval', async () => {
    const fetcher = vi.fn().mockResolvedValue(null)
    const { result } = renderHook(() => usePolling(fetcher, 10000))
    await flush()
    expect(fetcher).toHaveBeenCalledTimes(1)

    await act(async () => {
      result.current.refresh()
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('sets loading=true while the refreshed fetch is in flight', async () => {
    let resolve!: (v: unknown) => void
    const fetcher = vi.fn()
      .mockResolvedValueOnce({ n: 1 })
      .mockReturnValue(new Promise(r => { resolve = r }))

    const { result } = renderHook(() => usePolling(fetcher, 10000))
    await flush()
    expect(result.current.loading).toBe(false)

    act(() => { result.current.refresh() })
    expect(result.current.loading).toBe(true)

    await act(async () => {
      resolve({ n: 2 })
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(result.current.loading).toBe(false)
    expect(result.current.data).toEqual({ n: 2 })
  })
})

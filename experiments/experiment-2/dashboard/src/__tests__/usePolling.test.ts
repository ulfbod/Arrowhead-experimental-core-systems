import { renderHook, act } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'
import { usePolling } from '../hooks/usePolling'

describe('usePolling', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('calls fetcher immediately on mount', async () => {
    const fetcher = vi.fn().mockResolvedValue('data')
    renderHook(() => usePolling(fetcher, 5_000))
    await act(async () => {})
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('sets data on successful fetch', async () => {
    const fetcher = vi.fn().mockResolvedValue('hello')
    const { result } = renderHook(() => usePolling(fetcher, 5_000))
    await act(async () => {})
    expect(result.current.data).toBe('hello')
    expect(result.current.error).toBeNull()
    expect(result.current.loading).toBe(false)
  })

  it('sets error when fetcher rejects', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('network error'))
    const { result } = renderHook(() => usePolling(fetcher, 5_000))
    await act(async () => {})
    expect(result.current.error).toBe('network error')
    expect(result.current.data).toBeNull()
  })

  it('marks stale when previous data exists and next fetch fails', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce('first')
      .mockRejectedValueOnce(new Error('down'))
    const { result } = renderHook(() => usePolling(fetcher, 1_000))
    // First fetch succeeds.
    await act(async () => {})
    expect(result.current.data).toBe('first')
    // Second fetch (after interval) fails.
    await act(async () => { vi.advanceTimersByTime(1_000) })
    await act(async () => {})
    expect(result.current.stale).toBe(true)
    expect(result.current.data).toBe('first')  // keeps previous data
  })

  it('calls fetcher again after interval elapses', async () => {
    const fetcher = vi.fn().mockResolvedValue('x')
    renderHook(() => usePolling(fetcher, 1_000))
    await act(async () => {})
    expect(fetcher).toHaveBeenCalledTimes(1)
    await act(async () => { vi.advanceTimersByTime(1_000) })
    await act(async () => {})
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('stops polling after unmount', async () => {
    const fetcher = vi.fn().mockResolvedValue('x')
    const { unmount } = renderHook(() => usePolling(fetcher, 1_000))
    await act(async () => {})
    unmount()
    await act(async () => { vi.advanceTimersByTime(3_000) })
    // Only the initial call; no further calls after unmount.
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
})

import { useState, useEffect, useCallback, useRef } from 'react'
import axios from 'axios'

interface UsePollingResult<T> {
  data: T | null
  error: string | null
  loading: boolean
  refetch: () => void
}

/**
 * Custom hook that polls a URL every `interval` milliseconds.
 * Returns { data, error, loading, refetch }.
 *
 * @param url      - Fully-qualified URL (or empty string to disable)
 * @param interval - Poll interval in milliseconds
 * @param enabled  - Set to false to pause polling (default: true)
 */
function usePolling<T>(
  url: string,
  interval: number,
  enabled: boolean = true
): UsePollingResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState<boolean>(true)

  // Use a ref for the abort controller so we can cancel in-flight requests
  const abortRef = useRef<AbortController | null>(null)
  // Counter to force re-fetch without changing other deps
  const fetchCountRef = useRef(0)
  const [fetchTick, setFetchTick] = useState(0)

  const refetch = useCallback(() => {
    fetchCountRef.current += 1
    setFetchTick(t => t + 1)
  }, [])

  // Fetch function
  const fetchData = useCallback(async () => {
    if (!url || !enabled) return

    // Cancel any previous in-flight request
    if (abortRef.current) {
      abortRef.current.abort()
    }
    abortRef.current = new AbortController()

    try {
      const response = await axios.get<T>(url, {
        signal: abortRef.current.signal,
        timeout: Math.min(interval * 0.9, 8000),
      })
      setData(response.data)
      setError(null)
    } catch (err: unknown) {
      if (axios.isCancel(err)) return
      if (err instanceof Error) {
        if (err.name === 'AbortError' || err.name === 'CanceledError') return
        setError(err.message)
      } else {
        setError('Unknown error')
      }
    } finally {
      setLoading(false)
    }
  }, [url, interval, enabled])

  // Initial fetch + manual refetch trigger
  useEffect(() => {
    setLoading(true)
    fetchData()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchData, fetchTick])

  // Polling interval
  useEffect(() => {
    if (!url || !enabled) return

    const timer = setInterval(() => {
      fetchData()
    }, interval)

    return () => {
      clearInterval(timer)
      if (abortRef.current) {
        abortRef.current.abort()
      }
    }
  }, [url, interval, enabled, fetchData])

  return { data, error, loading, refetch }
}

export default usePolling

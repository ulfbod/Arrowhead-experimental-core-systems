import { useState, useEffect, useCallback, useRef } from 'react'

export interface PollState<T> {
  data:    T | null
  error:   string | null
  loading: boolean
  stale:   boolean
}

export function usePolling<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  intervalMs: number,
): PollState<T> & { refresh: () => void } {
  const [state, setState] = useState<PollState<T>>({
    data: null, error: null, loading: true, stale: false,
  })

  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher
  const abortRef = useRef<AbortController | null>(null)

  const run = useCallback(() => {
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl

    setState(prev => ({ ...prev, loading: true }))

    fetcherRef.current(ctrl.signal)
      .then(data => {
        if (ctrl.signal.aborted) return
        setState({ data, error: null, loading: false, stale: false })
      })
      .catch(err => {
        if (ctrl.signal.aborted) return
        const msg = err instanceof Error ? err.message : String(err)
        setState(prev => ({ ...prev, error: msg, loading: false, stale: prev.data !== null }))
      })
  }, [])

  useEffect(() => {
    run()
    const id = setInterval(run, intervalMs)
    return () => { clearInterval(id); abortRef.current?.abort() }
  }, [run, intervalMs])

  return { ...state, refresh: run }
}

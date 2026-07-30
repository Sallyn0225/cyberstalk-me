import { useEffect, useState } from 'react'

import { parseUsage, type UsageResponse, type UsageWindow } from '@/types/contract'

const USAGE_URL = '/api/v1/usage'

export interface UsageState {
  /** Null until the first successful load, and after any failure. */
  data: UsageResponse | null
  loading: boolean
  /** Set for a failed request, a non-2xx status, or an unparsable body. */
  error: string | null
}

/**
 * Usage statistics for one window. The second server-state source in the app,
 * next to `useDeviceStream` — deliberately plain `fetch` with no SSE and no
 * polling: aggregates change by the minute at most, and the tab is read once.
 *
 * Re-fetches whenever the window changes and aborts the in-flight request from
 * the cleanup, so a fast window switch cannot land an older response last.
 * Only mounted while the usage tab is open, which is what keeps the request off
 * the default page load.
 */
export function useUsage(usageWindow: UsageWindow): UsageState {
  const [state, setState] = useState<UsageState>({
    data: null,
    loading: true,
    error: null,
  })

  useEffect(() => {
    const controller = new AbortController()
    let cancelled = false

    // Keep the previous window's data on screen while the next one loads: the
    // charts read their shape from the payload itself, so what is shown stays
    // internally consistent, and the panel dims instead of collapsing.
    setState((prev) => ({ data: prev.data, loading: true, error: null }))

    async function load(): Promise<void> {
      try {
        const response = await fetch(
          `${USAGE_URL}?window=${encodeURIComponent(usageWindow)}`,
          { signal: controller.signal },
        )
        if (!response.ok) throw new Error(`usage responded ${response.status}`)
        const parsed = parseUsage(await response.json())
        if (cancelled) return
        if (parsed === null) {
          console.warn('discarded malformed usage payload')
          setState({ data: null, loading: false, error: '统计数据格式异常' })
          return
        }
        setState({ data: parsed, loading: false, error: null })
      } catch (err) {
        if (cancelled || controller.signal.aborted) return
        console.warn('usage fetch failed', err)
        // Drop the stale payload: showing the previous window's numbers under a
        // freshly selected window would be a lie.
        setState({ data: null, loading: false, error: '拉取使用时间统计失败' })
      }
    }

    void load()

    return () => {
      cancelled = true
      controller.abort()
    }
  }, [usageWindow])

  return state
}

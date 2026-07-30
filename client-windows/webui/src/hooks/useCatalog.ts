import { useEffect, useState } from 'react'
import { streamCatalog } from '@/lib/api'
import type { CatalogSnapshot } from '@/types/api'

/**
 * Subscribes to the catalog stream for the life of the page.
 *
 * The agent pushes a new snapshot whenever it has observed something, so the
 * discovery list updates on its own: switch to an application and it appears.
 */
export function useCatalog(): {
  snapshot: CatalogSnapshot | null
  error: string | null
} {
  const [snapshot, setSnapshot] = useState<CatalogSnapshot | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()

    streamCatalog((next) => {
      setSnapshot(next)
      setError(null)
    }, controller.signal).catch((cause: unknown) => {
      if (controller.signal.aborted) return
      setError(cause instanceof Error ? cause.message : '发现列表断开了')
    })

    return () => controller.abort()
  }, [])

  return { snapshot, error }
}

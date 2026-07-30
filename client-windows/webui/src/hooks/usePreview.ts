import { useEffect, useState } from 'react'
import { api } from '@/lib/api'
import type { Preview } from '@/types/api'

/**
 * Polls "what would be reported right now".
 *
 * The agent resolves this through the same mapping code the reporting loop
 * uses, against the rules currently in the draft. That is the point of the bar:
 * it is not a simulation of the rules, it is the rules.
 *
 * Polling rather than streaming because the answer depends on the draft as much
 * as on the foreground window, and a poll after each edit lands is simpler than
 * invalidating a stream.
 */
export function usePreview(intervalMs = 1000): Preview | null {
  const [preview, setPreview] = useState<Preview | null>(null)

  useEffect(() => {
    let cancelled = false

    const tick = async () => {
      try {
        const next = await api.getPreview()
        if (!cancelled) setPreview(next)
      } catch {
        // The bar keeps showing the last known answer. A transient failure is
        // not worth replacing a working preview with an error.
      }
    }

    void tick()
    const id = setInterval(() => void tick(), intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs])

  return preview
}

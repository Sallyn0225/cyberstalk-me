import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { emptyDraft } from '@/types/api'
import type { Draft, State, ValidationIssue } from '@/types/api'

/**
 * Owns the draft configuration.
 *
 * The browser edits a local copy for responsiveness and pushes it to the agent
 * on a short debounce. The agent is the authority: it holds the compiled rules
 * the live preview resolves against, and it decides whether the draft is valid.
 * Nothing here re-implements that judgement.
 */

const SYNC_DEBOUNCE_MS = 350

export interface DraftController {
  draft: Draft
  defaults: Draft
  configPath: string
  backupPath: string
  notice: string
  valid: boolean
  issue: ValidationIssue | null
  loading: boolean
  /** True when the draft differs from what is on disk. */
  dirty: boolean
  /** Failure to reach the agent, as opposed to a rejected configuration. */
  connectionError: string | null
  edit: (change: (draft: Draft) => Draft) => void
  /**
   * Pushes any pending edit immediately and reports what the agent made of it.
   * Call before saving: the agent saves the draft it holds, so an edit still
   * sitting in the debounce would not be in the file.
   *
   * Returns null when there was nothing to push, or when the push failed (in
   * which case connectionError says so).
   */
  flush: () => Promise<State | null>
  /** Records that the current draft is what is now on disk. */
  markSaved: () => void
  reload: () => Promise<void>
}

export function useDraft(): DraftController {
  const [state, setState] = useState<State | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [connectionError, setConnectionError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [dirty, setDirty] = useState(false)

  // The draft lives in a ref as well as in state: edits are applied against the
  // ref so that no side effect runs inside a setState updater, which React
  // may invoke more than once.
  const current = useRef<Draft>(emptyDraft)
  const pending = useRef<Draft | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // A snapshot of what is on disk, so "unsaved" is a fact rather than a guess
  // about whether any key was touched.
  const persisted = useRef<string>('')

  const adopt = useCallback((next: State) => {
    setState(next)
    setConnectionError(null)
  }, [])

  const reload = useCallback(async () => {
    try {
      const next = await api.getState()
      adopt(next)
      current.current = next.draft
      persisted.current = JSON.stringify(next.draft)
      setDraft(next.draft)
      setDirty(false)
    } catch (cause) {
      setConnectionError(cause instanceof Error ? cause.message : '读取配置失败')
    } finally {
      setLoading(false)
    }
  }, [adopt])

  useEffect(() => {
    void reload()
  }, [reload])

  const flush = useCallback(async (): Promise<State | null> => {
    if (timer.current) {
      clearTimeout(timer.current)
      timer.current = null
    }
    const next = pending.current
    if (!next) return null
    pending.current = null
    try {
      const state = await api.putDraft(next)
      adopt(state)
      return state
    } catch (cause) {
      // The edit stays pending so the next flush retries it rather than
      // dropping it on the floor.
      pending.current = next
      setConnectionError(cause instanceof Error ? cause.message : '同步配置失败')
      return null
    }
  }, [adopt])

  const edit = useCallback(
    (change: (draft: Draft) => Draft) => {
      const next = change(current.current)
      current.current = next
      setDraft(next)

      setDirty(JSON.stringify(next) !== persisted.current)

      pending.current = next
      if (timer.current) clearTimeout(timer.current)
      timer.current = setTimeout(() => void flush(), SYNC_DEBOUNCE_MS)
    },
    [flush],
  )

  // An edit still sitting in the debounce must not be lost on teardown.
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current)
    },
    [],
  )

  const markSaved = useCallback(() => {
    persisted.current = JSON.stringify(current.current)
    setDirty(false)
  }, [])

  return {
    draft,
    defaults: state?.defaults ?? emptyDraft,
    configPath: state?.config_path ?? '',
    backupPath: state?.backup_path ?? '',
    notice: state?.notice ?? '',
    valid: state?.valid ?? false,
    issue: state?.error ?? null,
    loading,
    dirty,
    connectionError,
    edit,
    flush,
    markSaved,
    reload,
  }
}

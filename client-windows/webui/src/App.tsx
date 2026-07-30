import { useCallback, useEffect, useState } from 'react'
import { CircleAlert, Info, Loader2, Save } from 'lucide-react'

import { AdvancedSettings } from '@/components/AdvancedSettings'
import { ConnectionSettings } from '@/components/ConnectionSettings'
import { DangerZone } from '@/components/DangerZone'
import { DiscoveryPanel } from '@/components/DiscoveryPanel'
import { PreviewBar } from '@/components/PreviewBar'
import { RuleEditor } from '@/components/RuleEditor'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/primitives'
import { useCatalog } from '@/hooks/useCatalog'
import { useDraft } from '@/hooks/useDraft'
import { usePreview } from '@/hooks/usePreview'
import { api } from '@/lib/api'
import { addPattern, addRule, newRule, normalizeProcess } from '@/lib/rules'

export default function App() {
  const config = useDraft()
  const { snapshot, error: catalogError } = useCatalog()
  const preview = usePreview()

  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [finished, setFinished] = useState(false)

  const { edit, flush, markSaved } = config
  const fields = config.issue?.fields ?? []

  /** Creates a rule for a process discovered in the catalog. */
  const handleAddRule = useCallback(
    (process: string) => {
      edit((draft) =>
        draft.rules.some((r) => normalizeProcess(r.process) === normalizeProcess(process))
          ? draft
          : addRule(draft, newRule(process)),
      )
    },
    [edit],
  )

  /**
   * Turns a title sample into a title_pattern for that process.
   *
   * The suggestion is escaped by the agent, with Go's rules, because Go is what
   * will run it.
   */
  const handleAddPattern = useCallback(
    async (process: string, title: string) => {
      let pattern = ''
      try {
        pattern = (await api.suggestPattern(title)).pattern
      } catch {
        // Without a suggestion an empty pattern is still a usable starting
        // point; the row shows what it currently matches either way.
      }
      edit((draft) => {
        const index = draft.rules.findIndex(
          (r) => normalizeProcess(r.process) === normalizeProcess(process),
        )
        return index < 0 ? draft : addPattern(draft, index, { match: pattern, description: '' })
      })
    },
    [edit],
  )

  const handleSave = useCallback(async () => {
    setSaving(true)
    setSaveError(null)
    setSaved(null)
    try {
      // Any edit still sitting in the debounce has to land first, or the agent
      // would save the configuration from a moment ago. The save button is
      // enabled off the last answer the agent gave, so the edit that is landing
      // right now may be the one that makes the draft unsaveable.
      const synced = await flush()
      if (synced && !synced.valid) {
        setSaveError(synced.error?.message ?? '这份配置还不能保存')
        return
      }
      const result = await api.save()
      if (result.saved) {
        markSaved()
        setSaved(result.backup_path ? `已保存，原文件备份到 ${result.backup_path}` : '已保存')
      } else {
        setSaveError(result.error?.message ?? '保存失败')
      }
    } catch (cause) {
      setSaveError(cause instanceof Error ? cause.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }, [flush, markSaved])

  const handleFinish = useCallback(async () => {
    setFinished(true)
    markSaved() // nothing left to warn about once the session is over
    try {
      await api.quit()
    } catch {
      // The agent closing the connection as it exits is the expected outcome.
    }
  }, [markSaved])

  useEffect(() => {
    if (!config.dirty) return
    // Closing the tab ends the session, and the agent's in-memory draft goes
    // with it. The browser shows its own wording; the event only opts in.
    const warn = (event: BeforeUnloadEvent) => event.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [config.dirty])

  if (config.loading) {
    return (
      <main className="grid min-h-[100dvh] place-items-center">
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden />
          正在读取配置
        </p>
      </main>
    )
  }

  if (finished) {
    return (
      <main className="grid min-h-[100dvh] place-items-center px-6">
        <div className="text-center">
          <h1 className="text-lg font-semibold">配置完成</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            agent 已经退出，这个页面可以关掉了。
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            接下来直接运行 <code className="font-mono">agent.exe</code> 就会开始上报。
          </p>
        </div>
      </main>
    )
  }

  return (
    <div className="flex min-h-[100dvh] flex-col">
      <header className="border-b border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-4">
          <div>
            <h1 className="text-base font-semibold">配置这台设备上报什么</h1>
            <p className="mt-1 font-mono text-xs text-muted-foreground" translate="no">
              {config.configPath}
            </p>
          </div>
          <SaveControls
            valid={config.valid}
            dirty={config.dirty}
            saving={saving}
            onSave={handleSave}
            onFinish={handleFinish}
          />
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-6 py-6">
        <Messages
          notice={config.notice}
          connectionError={config.connectionError}
          issue={config.valid ? null : (config.issue?.message ?? null)}
          saved={saved}
          saveError={saveError}
        />

        <div className="mt-6 grid gap-8 lg:grid-cols-[minmax(0,22rem)_minmax(0,1fr)]">
          <DiscoveryPanel
            snapshot={snapshot}
            error={catalogError}
            draft={config.draft}
            onAddRule={handleAddRule}
            onAddPattern={(process, title) => void handleAddPattern(process, title)}
          />

          <div className="flex flex-col gap-8">
            <RuleEditor draft={config.draft} edit={edit} fields={fields} />
            <ConnectionSettings
              draft={config.draft}
              defaults={config.defaults}
              edit={edit}
              fields={fields}
            />
            <AdvancedSettings
              draft={config.draft}
              defaults={config.defaults}
              edit={edit}
              fields={fields}
            />
            <DangerZone draft={config.draft} edit={edit} />
          </div>
        </div>
      </main>

      <PreviewBar preview={preview} exposed={config.draft.expose_title.length > 0} />
    </div>
  )
}

function SaveControls({
  valid,
  dirty,
  saving,
  onSave,
  onFinish,
}: {
  valid: boolean
  dirty: boolean
  saving: boolean
  onSave: () => void
  onFinish: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      {/* Edits reach the agent as you type, but only a save reaches the file.
          Without this, a closed tab looks like a saved configuration. */}
      {valid ? (
        dirty ? <Badge variant="warning">有改动没保存</Badge> : null
      ) : (
        <Badge variant="warning">还不能保存</Badge>
      )}
      <Button variant="outline" onClick={onSave} disabled={!valid || saving}>
        {saving ? <Loader2 className="animate-spin" aria-hidden /> : <Save aria-hidden />}
        保存
      </Button>
      <Button onClick={onFinish} disabled={!valid || saving}>
        完成并退出
      </Button>
    </div>
  )
}

/**
 * The message stack: why the draft is what it is, and what just happened.
 *
 * Ordered by urgency rather than by recency. A configuration that cannot be
 * saved matters more than a save that succeeded a moment ago.
 */
function Messages({
  notice,
  connectionError,
  issue,
  saved,
  saveError,
}: {
  notice: string
  connectionError: string | null
  issue: string | null
  saved: string | null
  saveError: string | null
}) {
  return (
    <div className="flex flex-col gap-2">
      {connectionError ? (
        <Message tone="destructive" text={connectionError} />
      ) : null}
      {saveError ? <Message tone="destructive" text={saveError} /> : null}
      {/* The agent's own words, kept verbatim so they match what it prints on
          startup, with a line of context because they are in English and the
          rest of this page is not. */}
      {issue ? <Message tone="warning" text={`agent 不会接受这份配置：${issue}`} /> : null}
      {notice ? <Message tone="info" text={notice} /> : null}
      {saved ? <Message tone="success" text={saved} /> : null}
    </div>
  )
}

function Message({
  tone,
  text,
}: {
  tone: 'destructive' | 'warning' | 'info' | 'success'
  text: string
}) {
  const styles = {
    destructive: 'border-destructive/40 bg-destructive/10 text-destructive',
    warning: 'border-warning/40 bg-warning/10 text-warning',
    info: 'border-border bg-muted/50 text-muted-foreground',
    success: 'border-primary/40 bg-primary/10 text-primary',
  }[tone]

  const Icon = tone === 'info' ? Info : CircleAlert

  return (
    <p
      role={tone === 'destructive' || tone === 'warning' ? 'alert' : 'status'}
      className={`flex items-start gap-2 rounded-md border px-3 py-2 text-xs ${styles}`}
    >
      <Icon className="mt-0.5 size-3.5 shrink-0" aria-hidden />
      <span className="break-words">{text}</span>
    </p>
  )
}

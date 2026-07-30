import { useEffect, useState } from 'react'
import { TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/primitives'
import { Input, Label } from '@/components/ui/field'
import { api } from '@/lib/api'
import { confirmationMatches, confirmationPhrase } from '@/lib/confirm'
import { exposesTitle, setExposeTitle } from '@/lib/rules'
import type { Draft } from '@/types/api'

/**
 * expose_title: the one and only way to switch sanitization off.
 *
 * A process listed here reports its raw window title verbatim, publicly, every
 * cycle. Everything else in this product exists to make sure that does not
 * happen by accident, so turning it on is deliberately awkward: a dialog that
 * shows the real, current title of that application, and a phrase to type.
 */
export function DangerZone({
  draft,
  edit,
}: {
  draft: Draft
  edit: (change: (draft: Draft) => Draft) => void
}) {
  const [pending, setPending] = useState<string | null>(null)
  const exposed = draft.expose_title

  if (draft.rules.length === 0 && exposed.length === 0) return null

  return (
    <section
      aria-labelledby="danger-heading"
      className="rounded-lg border border-destructive/40 bg-destructive/5 p-4"
    >
      <header className="flex items-start gap-2">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden />
        <div>
          <h2 id="danger-heading" className="text-sm font-semibold text-destructive">
            公开真实窗口标题
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            打开后，这个应用的窗口标题会一字不差地公开显示，包括文件名、网页标题、聊天对象。
            这是整个产品里唯一关掉脱敏的开关，默认一个都不开。
          </p>
        </div>
      </header>

      <ul className="mt-4 flex flex-col gap-2">
        {draft.rules
          .filter((rule) => rule.process.trim())
          .map((rule, index) => {
            const on = exposesTitle(draft, rule.process)
            return (
              <li
                // Position, not process name: two rules may briefly claim the
                // same process (the rule editor flags that rather than
                // preventing it), and duplicate keys drop a row.
                key={index}
                className="flex items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="truncate font-mono text-sm" translate="no">{rule.process}</p>
                  <p className="text-xs text-muted-foreground">
                    {on ? '正在公开真实标题' : `显示成「${rule.app || '未命名'}」`}
                  </p>
                </div>
                {on ? (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => edit((d) => setExposeTitle(d, rule.process, false))}
                  >
                    关掉
                  </Button>
                ) : (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive hover:bg-destructive/10"
                    onClick={() => setPending(rule.process)}
                  >
                    公开标题
                  </Button>
                )}
              </li>
            )
          })}
      </ul>

      <ExposeTitleDialog
        process={pending}
        onCancel={() => setPending(null)}
        onConfirm={(process) => {
          edit((d) => setExposeTitle(d, process, true))
          setPending(null)
        }}
      />
    </section>
  )
}

/**
 * The confirmation gate.
 *
 * It shows the application's actual current window title, live. A warning that
 * says "your titles will be public" is abstract; seeing the words that are
 * about to be published is not.
 */
function ExposeTitleDialog({
  process,
  onCancel,
  onConfirm,
}: {
  process: string | null
  onCancel: () => void
  onConfirm: (process: string) => void
}) {
  const [typed, setTyped] = useState('')
  const samples = useLiveTitles(process)

  useEffect(() => {
    setTyped('')
  }, [process])

  if (!process) return null
  const ready = confirmationMatches(typed, process)

  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent>
        <DialogTitle className="text-destructive">
          公开 {process} 的真实窗口标题？
        </DialogTitle>
        <DialogDescription>
          打开之后，下面这些标题会原样出现在公开页面上，每次上报都是。
        </DialogDescription>

        <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3">
          <p className="text-xs font-medium text-destructive">
            这个应用最近的真实标题
          </p>
          {samples === null ? (
            <p className="mt-2 text-xs text-muted-foreground">读取中</p>
          ) : samples.length === 0 ? (
            <p className="mt-2 text-xs text-muted-foreground">
              还没见过这个应用的窗口标题。切过去用一下，再回来确认。
            </p>
          ) : (
            <ul className="mt-2 flex flex-col gap-1">
              {samples.slice(0, 5).map((title) => (
                <li key={title} className="font-mono text-xs break-words" translate="no">
                  {title}
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="grid gap-2">
          <Label htmlFor="expose-confirm">
            确认请输入 <span className="font-mono" translate="no">{confirmationPhrase(process)}</span>
          </Label>
          <Input
            id="expose-confirm"
            value={typed}
            spellCheck={false}
            autoComplete="off"
            className="font-mono"
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>

        <div className="flex justify-end gap-2">
          <DialogClose asChild>
            <Button variant="outline">取消</Button>
          </DialogClose>
          <Button
            variant="destructive"
            disabled={!ready}
            onClick={() => onConfirm(process)}
          >
            公开标题
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}

/**
 * The titles the agent has actually seen for this process.
 *
 * Fetched through the regexp tester with a pattern that matches everything,
 * which is the endpoint that already returns the sample list for one process.
 */
function useLiveTitles(process: string | null): string[] | null {
  const [titles, setTitles] = useState<string[] | null>(null)

  useEffect(() => {
    if (!process) {
      setTitles(null)
      return
    }
    let cancelled = false

    const load = async () => {
      try {
        const result = await api.testPattern('', process)
        if (!cancelled) setTitles(result.titles ?? [])
      } catch {
        if (!cancelled) setTitles([])
      }
    }

    void load()
    // Kept live while the dialog is open: if the user switches to the app to
    // see what its titles look like, the dialog updates.
    const id = setInterval(() => void load(), 2000)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [process])

  return titles
}

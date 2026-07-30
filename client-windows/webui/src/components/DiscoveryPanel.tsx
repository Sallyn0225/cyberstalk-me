import { useMemo, useState } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ChevronRight, Lock, Plus, ShieldAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Badge,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/primitives'
import { normalizeProcess } from '@/lib/rules'
import { isRecent, relativeTime } from '@/lib/time'
import { cn } from '@/lib/utils'
import type { CatalogApp, CatalogSnapshot, Draft } from '@/types/api'

/**
 * The applications the agent has watched you use, and the window titles it saw.
 *
 * Nothing is enumerated: this list is built purely from what has been in the
 * foreground since the session started. Switch to the application you want to
 * configure and it appears here, which is also why the list is ordered by
 * recency.
 */
export function DiscoveryPanel({
  snapshot,
  error,
  draft,
  onAddRule,
  onAddPattern,
}: {
  snapshot: CatalogSnapshot | null
  error: string | null
  draft: Draft
  onAddRule: (process: string) => void
  onAddPattern: (process: string, title: string) => void
}) {
  const apps = snapshot?.apps ?? []
  const configured = useMemo(
    () => new Set(draft.rules.map((rule) => normalizeProcess(rule.process))),
    [draft.rules],
  )

  return (
    <section aria-labelledby="discovery-heading" className="flex min-h-0 flex-col gap-4">
      <header className="flex items-baseline justify-between gap-4">
        <h2 id="discovery-heading" className="text-sm font-semibold">
          用过的应用
        </h2>
        <p className="text-xs text-muted-foreground">
          切到想配的应用，它就会出现在这里
        </p>
      </header>

      {error ? (
        <p role="alert" className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {error}
        </p>
      ) : null}

      {snapshot?.locked ? (
        <p className="flex items-center gap-2 rounded-md border border-border bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
          <Lock className="size-3.5 shrink-0" aria-hidden />
          现在是锁屏状态。锁屏期间不读窗口标题，也不会发现新应用。
        </p>
      ) : null}

      {apps.length === 0 ? (
        <EmptyState waiting={snapshot !== null} />
      ) : (
        <ul className="flex flex-col gap-2">
          <AnimatePresence initial={false}>
            {apps.map((app) => (
              <AppRow
                key={app.process}
                app={app}
                configured={configured.has(normalizeProcess(app.process))}
                onAddRule={() => onAddRule(app.process)}
                onAddPattern={(title) => onAddPattern(app.process, title)}
              />
            ))}
          </AnimatePresence>
        </ul>
      )}
    </section>
  )
}

function EmptyState({ waiting }: { waiting: boolean }) {
  return (
    <div className="rounded-lg border border-dashed border-border px-4 py-8 text-center">
      <p className="text-sm text-muted-foreground">
        {waiting ? '还没看到任何应用' : '正在连接 agent'}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">
        切到另一个窗口再切回来，这里就会有东西
      </p>
    </div>
  )
}

function AppRow({
  app,
  configured,
  onAddRule,
  onAddPattern,
}: {
  app: CatalogApp
  configured: boolean
  onAddRule: () => void
  onAddPattern: (title: string) => void
}) {
  const [open, setOpen] = useState(false)
  const reduce = useReducedMotion()
  const samples = app.samples ?? []
  const now = Date.now()
  const fresh = isRecent(app.first_seen, now, 4000)

  return (
    <motion.li
      layout={reduce ? false : 'position'}
      initial={reduce ? false : { opacity: 0, y: -6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.22, ease: [0.16, 1, 0.3, 1] }}
      className={cn(
        'rounded-lg border bg-card',
        // A newly discovered application is highlighted briefly: the whole
        // interaction is "switch to it and look here".
        fresh ? 'border-primary/60' : 'border-border',
      )}
    >
      <Collapsible open={open} onOpenChange={setOpen}>
        <div className="flex items-center gap-2 p-3">
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 shrink-0 gap-1 px-1.5 text-muted-foreground"
              disabled={samples.length === 0}
            >
              <ChevronRight
                className={cn('size-3.5 transition-transform', open && 'rotate-90')}
                aria-hidden
              />
              <span className="sr-only">
                {open ? '收起标题样本' : '展开标题样本'}
              </span>
              <span aria-hidden className="text-xs tabular-nums">
                {samples.length}
              </span>
            </Button>
          </CollapsibleTrigger>

          <div className="min-w-0 flex-1">
            <p className="truncate font-mono text-sm" translate="no">{app.process}</p>
            <p className="text-xs text-muted-foreground">
              {relativeTime(app.last_seen, now)} · 出现 {app.count} 次
            </p>
          </div>

          {!app.configurable ? (
            <Badge variant="warning" title="提权窗口拿不到进程名，没法为它写规则">
              <ShieldAlert aria-hidden />
              无法配置
            </Badge>
          ) : configured ? (
            <Badge variant="secondary">已有规则</Badge>
          ) : (
            <Button size="sm" variant="outline" onClick={onAddRule}>
              <Plus aria-hidden />
              加规则
            </Button>
          )}
        </div>

        <CollapsibleContent>
          {samples.length > 0 ? (
            <ul className="border-t border-border">
              {samples.map((sample) => (
                <li
                  key={sample.title}
                  className="flex items-start gap-2 px-3 py-2 hover:bg-muted/40"
                >
                  <div className="min-w-0 flex-1">
                    {/* A window title can be a whole sentence (a page title, a
                        repository description). Two lines is enough to
                        recognize it; the full text is one hover away. */}
                    <p
                      className="line-clamp-2 font-mono text-xs break-words"
                      translate="no"
                      title={sample.title}
                    >
                      {sample.title}
                    </p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {relativeTime(sample.last_seen, now)} · {sample.count} 次
                    </p>
                  </div>
                  {configured && app.configurable ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="shrink-0"
                      onClick={() => onAddPattern(sample.title)}
                    >
                      按这条加细分
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </CollapsibleContent>
      </Collapsible>
    </motion.li>
  )
}

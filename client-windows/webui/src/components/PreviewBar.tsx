import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { Eye, Lock, MoonStar, ShieldQuestion } from 'lucide-react'

import { Badge } from '@/components/ui/primitives'
import { cn } from '@/lib/utils'
import type { Preview } from '@/types/api'

/**
 * What the site would show right now, given the rules currently being edited.
 *
 * The agent computes this with the same code that produces a real report, so
 * this is not an approximation of the rules. It is the answer to the only
 * question that matters while writing them: "what does the world see?"
 */
export function PreviewBar({
  preview,
  exposed,
}: {
  preview: Preview | null
  exposed: boolean
}) {
  const reduce = useReducedMotion()
  const activity = preview?.activity

  return (
    <div className="sticky bottom-0 z-30 border-t border-border bg-background/95 backdrop-blur">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-4 gap-y-2 px-6 py-3">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Eye className="size-4" aria-hidden />
          <span>此刻会显示成</span>
        </div>

        <div className="flex min-h-7 flex-1 items-center gap-2" aria-live="polite">
          {activity ? (
            <AnimatePresence mode="popLayout" initial={false}>
              <motion.div
                // Keyed on the rendered result so the bar animates when the
                // answer changes, not on every poll that returns the same thing.
                key={`${activity.app}|${activity.description}`}
                initial={reduce ? false : { opacity: 0, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                exit={reduce ? undefined : { opacity: 0, y: -4 }}
                transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
                className="flex flex-wrap items-center gap-2"
              >
                <span className="text-sm font-medium">{activity.app}</span>
                <span className="text-sm text-muted-foreground">{activity.description}</span>
              </motion.div>
            </AnimatePresence>
          ) : (
            <span className="text-sm text-muted-foreground">读取中</span>
          )}
        </div>

        <div className="flex items-center gap-2">
          {exposed ? (
            <Badge variant="destructive">
              <ShieldQuestion aria-hidden />
              正在公开真实标题
            </Badge>
          ) : null}
          {activity?.locked ? (
            <Badge variant="outline">
              <Lock aria-hidden />
              锁屏中
            </Badge>
          ) : null}
          {activity?.idle && !activity.locked ? (
            <Badge variant="outline">
              <MoonStar aria-hidden />
              空闲 {formatIdle(activity.idle_seconds)}
            </Badge>
          ) : null}
          <span
            translate="no"
            className={cn(
              'font-mono text-xs',
              preview?.process ? 'text-muted-foreground' : 'text-muted-foreground/60',
            )}
          >
            {preview?.process || '无前台窗口'}
          </span>
        </div>
      </div>
    </div>
  )
}

function formatIdle(seconds: number): string {
  if (seconds < 60) return `${seconds} 秒`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟`
  return `${Math.floor(minutes / 60)} 小时`
}

import { useState } from 'react'
import { ChevronRight } from 'lucide-react'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { formatDuration } from '@/lib/format'
import { maxSeconds, sharePercent } from '@/lib/usage'
import type { AppUsage } from '@/types/contract'

/** How many apps show before the tail is folded away. */
const COLLAPSED_ROWS = 8

interface UsageAppListProps {
  /** Active-time ranking from the server, already descending. */
  apps: AppUsage[]
}

/**
 * Two-level active-time ranking: one row per app, expanding into the mapped
 * descriptions inside it.
 *
 * Both levels render the seconds the server sent. The per-description numbers
 * already sum to the app total, so nothing is re-summed here — if the two ever
 * disagree, that is a server bug and hiding it in the UI would only delay it.
 */
export function UsageAppList({ apps }: UsageAppListProps) {
  const [showAll, setShowAll] = useState(false)

  if (apps.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">这段时间没有活跃的应用记录。</p>
    )
  }

  // Bars are scaled against the longest app rather than the total, so the
  // ranking stays readable when one app dominates the window.
  const longest = maxSeconds(apps)
  const visible = showAll ? apps : apps.slice(0, COLLAPSED_ROWS)

  return (
    <div className="flex flex-col gap-3">
      <ul className="flex flex-col gap-3">
        {visible.map((app) => (
          <li key={app.app}>
            <Collapsible>
              {/* One line per app on sm and up: name, bar, duration. Below that
                  the bar wraps under the label instead of being squeezed. */}
              <CollapsibleTrigger className="group/app focus-visible:ring-ring/50 flex w-full cursor-pointer flex-wrap items-center gap-x-2 gap-y-1.5 rounded-md text-left transition-opacity hover:opacity-80 focus-visible:ring-[3px] focus-visible:outline-none">
                <ChevronRight
                  aria-hidden
                  className="text-muted-foreground order-1 size-3.5 shrink-0 transition-transform group-data-[state=open]/app:rotate-90"
                />
                <span
                  className="order-2 min-w-0 flex-1 truncate text-sm font-medium sm:flex-none sm:basis-52"
                  title={app.app}
                >
                  {app.app.trim() || '未知应用'}
                </span>
                <span
                  aria-hidden
                  className="bg-muted/50 order-4 block h-1.5 w-full overflow-hidden rounded-full sm:order-3 sm:w-auto sm:flex-1"
                >
                  <span
                    className="bg-primary block h-full rounded-full"
                    style={{ width: `${sharePercent(app.seconds, longest)}%` }}
                  />
                </span>
                <span className="text-muted-foreground order-3 shrink-0 text-sm tabular-nums sm:order-4 sm:w-24 sm:text-right">
                  {formatDuration(app.seconds)}
                </span>
              </CollapsibleTrigger>

              <CollapsibleContent className="data-[state=open]:animate-in data-[state=open]:fade-in-0 motion-reduce:animate-none">
                <ul className="border-border mt-2 ml-1.5 flex flex-col gap-2 border-l pl-4">
                  {app.activities.map((activity) => (
                    <li
                      key={activity.description}
                      className="flex flex-wrap items-center gap-x-2 gap-y-1"
                    >
                      {/* basis matches the app row minus this list's indent, so
                          the nested bars start on the same x as their parent. */}
                      <span
                        className="text-muted-foreground order-1 min-w-0 flex-1 truncate text-xs sm:flex-none sm:basis-52"
                        title={activity.description}
                      >
                        {activity.description.trim() || '未标注'}
                      </span>
                      <span
                        aria-hidden
                        className="bg-muted/40 order-3 block h-1 w-full overflow-hidden rounded-full sm:order-2 sm:w-auto sm:flex-1"
                      >
                        <span
                          className="bg-primary/50 block h-full rounded-full"
                          style={{
                            width: `${sharePercent(activity.seconds, app.seconds)}%`,
                          }}
                        />
                      </span>
                      <span className="text-muted-foreground order-2 shrink-0 text-xs tabular-nums sm:order-3 sm:w-24 sm:text-right">
                        {formatDuration(activity.seconds)}
                      </span>
                    </li>
                  ))}
                </ul>
              </CollapsibleContent>
            </Collapsible>
          </li>
        ))}
      </ul>

      {apps.length > COLLAPSED_ROWS ? (
        <button
          type="button"
          onClick={() => setShowAll((prev) => !prev)}
          className="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 w-fit cursor-pointer rounded-md text-xs transition-colors focus-visible:ring-[3px] focus-visible:outline-none"
        >
          {showAll ? '收起' : `展开剩下的 ${apps.length - COLLAPSED_ROWS} 个应用`}
        </button>
      ) : null}
    </div>
  )
}

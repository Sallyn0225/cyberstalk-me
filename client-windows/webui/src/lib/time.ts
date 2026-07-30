/**
 * Relative timestamps for the discovery panel.
 *
 * The catalog's value is recency: "you were in this window a second ago" is
 * what makes the list make sense. An absolute clock time would make the reader
 * do the subtraction.
 */

/**
 * relativeTime renders how long ago iso was, from the caller's "now".
 *
 * now is a parameter rather than a call to Date.now() so this stays pure and
 * testable, and so a list of twenty entries renders against one consistent
 * instant instead of twenty slightly different ones.
 */
export function relativeTime(iso: string, now: number): string {
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return ''

  const seconds = Math.round((now - then) / 1000)
  if (seconds < 0) return '刚刚' // clock skew; "in the future" would read as a bug
  if (seconds < 5) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`

  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`

  return `${Math.floor(hours / 24)} 天前`
}

/** isRecent reports whether a timestamp is new enough to highlight. */
export function isRecent(iso: string, now: number, withinMs = 3000): boolean {
  const then = Date.parse(iso)
  if (Number.isNaN(then)) return false
  return now - then <= withinMs
}

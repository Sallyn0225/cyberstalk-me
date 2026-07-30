/**
 * Parses the snippet printed by `server register-device`.
 *
 * That command prints the four connection keys in config.yaml's own syntax, so
 * the fastest way to fill this form is to paste the whole block. Retyping a
 * 64-character token by hand is how people end up with a device that silently
 * never reports.
 */

/** The connection keys a pasted snippet can carry. */
export interface ParsedConnection {
  server_url?: string
  device_id?: string
  token?: string
  interval?: string
}

const KEYS = ['server_url', 'device_id', 'token', 'interval'] as const

/**
 * parseRegisterSnippet pulls whatever connection keys it recognizes out of
 * pasted text. Unknown lines are ignored rather than rejected: the command
 * prints prose around the snippet, and a paste that includes it should still
 * work.
 *
 * Returns null when nothing was recognized, which is what tells the UI to leave
 * the field alone and let the paste land as ordinary text.
 */
export function parseRegisterSnippet(text: string): ParsedConnection | null {
  const found: ParsedConnection = {}

  for (const rawLine of text.split(/\r?\n/)) {
    // Strip a YAML comment, but not a "#" inside a quoted value.
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue

    const separator = line.indexOf(':')
    if (separator < 0) continue

    const key = line.slice(0, separator).trim().toLowerCase()
    if (!(KEYS as readonly string[]).includes(key)) continue

    const value = unquote(line.slice(separator + 1).trim())
    if (value) found[key as keyof ParsedConnection] = value
  }

  return Object.keys(found).length > 0 ? found : null
}

/**
 * unquote removes surrounding quotes and any trailing comment.
 *
 * A URL contains "//" and a token can contain anything, so the comment strip
 * only applies to an unquoted value with whitespace before the "#" - the same
 * rule YAML itself uses.
 */
function unquote(value: string): string {
  const quoted = /^(["'])(.*)\1$/.exec(value)
  if (quoted) return quoted[2]

  const comment = value.search(/\s#/)
  return (comment >= 0 ? value.slice(0, comment) : value).trim()
}

import type {
  CatalogSnapshot,
  Draft,
  Preview,
  RegexTestResult,
  SaveResult,
  State,
} from '@/types/api'

/**
 * The client for the agent's local API.
 *
 * Every request carries this session's bearer token. The token arrives embedded
 * in the page the agent served, never in the URL, so it stays out of browser
 * history and out of any Referer header.
 */

declare global {
  interface Window {
    __SETUP_TOKEN__?: string
  }
}

/**
 * sessionToken reads the token the agent embedded in the page.
 *
 * In dev the page comes from the Vite server instead, which has nothing to
 * embed, so the token is taken from the query string. That branch is compiled
 * out of the production bundle: `import.meta.env.DEV` is a literal false there,
 * and the URL is never a token source in a shipped build.
 */
function sessionToken(): string {
  const embedded = window.__SETUP_TOKEN__
  if (embedded && !embedded.startsWith('__SETUP_TOKEN')) return embedded

  if (import.meta.env.DEV) {
    return new URLSearchParams(window.location.search).get('token') ?? ''
  }
  return ''
}

/** ApiError carries the status so callers can tell "rejected" from "broken". */
export class ApiError extends Error {
  // Declared as a field rather than a parameter property: the project builds
  // with erasableSyntaxOnly, which rules out constructor-parameter shorthand.
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Authorization', `Bearer ${sessionToken()}`)
  if (init.body !== undefined) headers.set('Content-Type', 'application/json')

  let response: Response
  try {
    response = await fetch(path, { ...init, headers })
  } catch {
    // The agent exiting mid-session is the common case here, not a network
    // outage: there is no network involved.
    throw new ApiError('连不上 agent，它可能已经退出了', 0)
  }

  if (!response.ok) {
    throw new ApiError(await errorMessage(response), response.status)
  }
  return (await response.json()) as T
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { error?: string }
    if (body.error) return body.error
  } catch {
    // A non-JSON body is not worth surfacing; the status is the information.
  }
  if (response.status === 401) return '会话令牌无效，刷新页面试试'
  if (response.status === 403) return '请求被拒绝：这个页面不是 agent 打开的那个'
  return `请求失败（HTTP ${response.status}）`
}

export const api = {
  getState: () => request<State>('/api/config'),

  putDraft: (draft: Draft) =>
    request<State>('/api/config', { method: 'PUT', body: JSON.stringify(draft) }),

  getPreview: () => request<Preview>('/api/preview'),

  testPattern: (pattern: string, process: string) =>
    request<RegexTestResult>('/api/regex/test', {
      method: 'POST',
      body: JSON.stringify({ pattern, process }),
    }),

  suggestPattern: (title: string) =>
    request<{ pattern: string }>('/api/regex/suggest', {
      method: 'POST',
      body: JSON.stringify({ title }),
    }),

  save: () => request<SaveResult>('/api/save', { method: 'POST' }),

  quit: () => request<{ stopping: boolean }>('/api/quit', { method: 'POST' }),
}

/**
 * streamCatalog subscribes to the catalog and calls onSnapshot for every frame.
 *
 * This reads the stream with fetch rather than EventSource because EventSource
 * cannot set request headers, and the session token must travel in
 * Authorization rather than in the URL. The framing the agent sends is plain
 * SSE: "data: {json}\n\n".
 *
 * Returns when the stream ends or the signal aborts.
 */
export async function streamCatalog(
  onSnapshot: (snapshot: CatalogSnapshot) => void,
  signal: AbortSignal,
): Promise<void> {
  const response = await fetch('/api/catalog', {
    headers: { Authorization: `Bearer ${sessionToken()}` },
    signal,
  })
  if (!response.ok) {
    throw new ApiError(await errorMessage(response), response.status)
  }
  if (!response.body) {
    throw new ApiError('这个浏览器不支持流式响应', 0)
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) return
      buffer += decoder.decode(value, { stream: true })

      // Frames are separated by a blank line; anything after the last one is a
      // partial frame that the next read completes.
      let boundary = buffer.indexOf('\n\n')
      while (boundary >= 0) {
        const frame = buffer.slice(0, boundary)
        buffer = buffer.slice(boundary + 2)
        const payload = frame.startsWith('data: ') ? frame.slice(6) : ''
        if (payload) {
          try {
            onSnapshot(JSON.parse(payload) as CatalogSnapshot)
          } catch {
            // A frame that does not parse is dropped rather than tearing the
            // stream down: the next one is a second away.
          }
        }
        boundary = buffer.indexOf('\n\n')
      }
    }
  } finally {
    reader.cancel().catch(() => {})
  }
}

import { useState } from 'react'
import { ClipboardCheck, Eye, EyeOff } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Field, Input } from '@/components/ui/field'
import { parseRegisterSnippet } from '@/lib/register'
import type { Draft } from '@/types/api'

/**
 * Where to report, and as whom.
 *
 * `server register-device` prints these four keys in config.yaml's own syntax,
 * so pasting its output into any of these fields fills all four. Retyping a
 * 64-character token by hand is how a device ends up silently never reporting.
 */
export function ConnectionSettings({
  draft,
  defaults,
  edit,
  fields,
}: {
  draft: Draft
  defaults: Draft
  edit: (change: (draft: Draft) => Draft) => void
  fields: string[]
}) {
  const [showToken, setShowToken] = useState(false)
  const [pasted, setPasted] = useState<string[] | null>(null)
  const flagged = (key: string) => fields.includes(key)

  /** Intercepts a paste that looks like the register-device snippet. */
  const handlePaste = (event: React.ClipboardEvent) => {
    const text = event.clipboardData.getData('text')
    const parsed = parseRegisterSnippet(text)
    // A single value pasted into a single field is an ordinary paste; only a
    // snippet carrying real keys takes over.
    if (!parsed) return

    event.preventDefault()
    edit((d) => ({ ...d, ...parsed }))
    setPasted(Object.keys(parsed))
  }

  return (
    <section aria-labelledby="connection-heading" className="flex flex-col gap-4">
      <header>
        <h2 id="connection-heading" className="text-sm font-semibold">
          连接
        </h2>
        <p className="mt-1 text-xs text-muted-foreground">
          把 <code className="font-mono">server register-device</code> 打印的那几行整段粘进任意一格，会自动拆开填好
        </p>
      </header>

      <form
        className="grid gap-4 sm:grid-cols-2"
        onPaste={handlePaste}
        // Nothing is submitted anywhere; the agent is updated as you type.
        onSubmit={(e) => e.preventDefault()}
      >
        <Field
          label="服务端地址"
          htmlFor="server-url"
          error={flagged('server_url') ? '需要形如 http://host:port 的地址，末尾不带路径' : undefined}
        >
          <Input
            id="server-url"
            value={draft.server_url}
            spellCheck={false}
            placeholder="http://localhost:8080"
            aria-invalid={flagged('server_url')}
            onChange={(e) => edit((d) => ({ ...d, server_url: e.target.value }))}
          />
        </Field>

        <Field
          label="设备 ID"
          htmlFor="device-id"
          error={flagged('device_id') ? '设备 ID 不能为空' : undefined}
        >
          <Input
            id="device-id"
            value={draft.device_id}
            spellCheck={false}
            className="font-mono"
            placeholder="win-desktop"
            aria-invalid={flagged('device_id')}
            onChange={(e) => edit((d) => ({ ...d, device_id: e.target.value }))}
          />
        </Field>

        <Field
          label="设备令牌"
          htmlFor="device-token"
          hint="注册设备时打印的那串，它是密码"
          error={flagged('token') ? '令牌不能为空' : undefined}
          className="sm:col-span-2"
        >
          <div className="flex gap-2">
            <Input
              id="device-token"
              // Masked by default so a screen share or a screenshot does not
              // hand out the token.
              type={showToken ? 'text' : 'password'}
              value={draft.token}
              spellCheck={false}
              autoComplete="off"
              className="font-mono"
              aria-invalid={flagged('token')}
              onChange={(e) => edit((d) => ({ ...d, token: e.target.value }))}
            />
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="shrink-0"
              onClick={() => setShowToken((v) => !v)}
            >
              {showToken ? <EyeOff aria-hidden /> : <Eye aria-hidden />}
              <span className="sr-only">{showToken ? '隐藏令牌' : '显示令牌'}</span>
            </Button>
          </div>
        </Field>

        <Field
          label="上报间隔"
          htmlFor="interval"
          hint={`留空按 ${defaults.interval}`}
          error={flagged('interval') ? '写法形如 10s、1m30s，或直接写秒数' : undefined}
        >
          <Input
            id="interval"
            value={draft.interval}
            spellCheck={false}
            placeholder={defaults.interval}
            aria-invalid={flagged('interval')}
            onChange={(e) => edit((d) => ({ ...d, interval: e.target.value }))}
          />
        </Field>
      </form>

      {pasted ? (
        <p className="flex items-center gap-2 text-xs text-primary">
          <ClipboardCheck className="size-3.5" aria-hidden />
          已从粘贴内容里认出 {pasted.length} 个字段
        </p>
      ) : null}
    </section>
  )
}

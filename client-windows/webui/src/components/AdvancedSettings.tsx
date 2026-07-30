import { useEffect, useState } from 'react'
import { ChevronRight } from 'lucide-react'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/primitives'
import { Field, Input } from '@/components/ui/field'
import { cn } from '@/lib/utils'
import type { Draft } from '@/types/api'

/** The YAML keys this panel is responsible for showing. */
const OWNED_FIELDS = [
  'device_name',
  'idle_threshold',
  'default_app',
  'default_description',
  'locked_app',
  'locked_description',
]

/**
 * The remaining keys, folded away because their defaults are already right.
 *
 * The two "default" fields are the privacy floor: they are what an application
 * with no rule reports. They are deliberately vague, and the placeholder shows
 * what they fall back to when cleared.
 */
export function AdvancedSettings({
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
  const [open, setOpen] = useState(false)

  // A complaint about a key that lives in here is useless while it is folded
  // away: the user is told something is wrong and cannot see what.
  const hasFlaggedField = fields.some((field) => OWNED_FIELDS.includes(field))
  useEffect(() => {
    if (hasFlaggedField) setOpen(true)
  }, [hasFlaggedField])

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger className="flex w-full items-center gap-2 rounded-md py-2 text-left text-sm font-semibold outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
        <ChevronRight
          className={cn('size-4 text-muted-foreground transition-transform', open && 'rotate-90')}
          aria-hidden
        />
        其它设置
        <span className="text-xs font-normal text-muted-foreground">
          没配的应用怎么显示、锁屏怎么显示、多久算空闲
        </span>
      </CollapsibleTrigger>

      <CollapsibleContent>
        <div className="grid gap-4 pt-2 pb-1 sm:grid-cols-2">
          <Field
            label="设备名"
            htmlFor="device-name"
            hint="只影响本机日志，站点上显示的名字由服务端登记时决定"
          >
            <Input
              id="device-name"
              value={draft.device_name}
              placeholder="我的台式机"
              onChange={(e) => edit((d) => ({ ...d, device_name: e.target.value }))}
            />
          </Field>

          <Field
            label="多久没操作算空闲"
            htmlFor="idle-threshold"
            hint={`留空按 ${defaults.idle_threshold}`}
            error={
              fields.includes('idle_threshold')
                ? '写法形如 5m、90s，或直接写秒数'
                : undefined
            }
          >
            <Input
              id="idle-threshold"
              value={draft.idle_threshold}
              spellCheck={false}
              placeholder={defaults.idle_threshold}
              aria-invalid={fields.includes('idle_threshold')}
              onChange={(e) => edit((d) => ({ ...d, idle_threshold: e.target.value }))}
            />
          </Field>

          <Field
            label="没配规则的应用显示成"
            htmlFor="default-app"
            hint="故意笼统：exe 名字本身就可能泄露信息"
          >
            <Input
              id="default-app"
              value={draft.default_app}
              placeholder={defaults.default_app}
              onChange={(e) => edit((d) => ({ ...d, default_app: e.target.value }))}
            />
          </Field>

          <Field label="没配规则时在干什么" htmlFor="default-description">
            <Input
              id="default-description"
              value={draft.default_description}
              placeholder={defaults.default_description}
              onChange={(e) =>
                edit((d) => ({ ...d, default_description: e.target.value }))
              }
            />
          </Field>

          <Field label="锁屏时显示成" htmlFor="locked-app">
            <Input
              id="locked-app"
              value={draft.locked_app}
              placeholder={defaults.locked_app}
              onChange={(e) => edit((d) => ({ ...d, locked_app: e.target.value }))}
            />
          </Field>

          <Field label="锁屏时在干什么" htmlFor="locked-description">
            <Input
              id="locked-description"
              value={draft.locked_description}
              placeholder={defaults.locked_description}
              onChange={(e) =>
                edit((d) => ({ ...d, locked_description: e.target.value }))
              }
            />
          </Field>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

import { useEffect, useState } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/primitives'
import { Field, Input } from '@/components/ui/field'
import { api } from '@/lib/api'
import {
  addPattern,
  duplicateProcesses,
  movePattern,
  newRule,
  normalizeProcess,
  patternsOf,
  removePattern,
  removeRule,
  updatePattern,
  updateRule,
} from '@/lib/rules'
import { cn } from '@/lib/utils'
import type { Draft, Rule } from '@/types/api'

/**
 * The rules: what each application is called publicly, and what you are said to
 * be doing in it.
 *
 * A process with no rule reports the generic default, never its executable
 * name. Writing a rule is the deliberate act of making an application public.
 */
export function RuleEditor({
  draft,
  edit,
  fields,
}: {
  draft: Draft
  edit: (change: (draft: Draft) => Draft) => void
  /** YAML paths the agent flagged, e.g. "rules[1].app". */
  fields: string[]
}) {
  const duplicates = duplicateProcesses(draft)
  const reduce = useReducedMotion()

  return (
    <section aria-labelledby="rules-heading" className="flex flex-col gap-4">
      <header className="flex items-center justify-between gap-4">
        <div>
          <h2 id="rules-heading" className="text-sm font-semibold">
            规则
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            没有规则的应用只会显示成「{draft.default_app || '某个应用'}」
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => edit((d) => ({ ...d, rules: [...d.rules, newRule('')] }))}
        >
          <Plus aria-hidden />
          手动添加
        </Button>
      </header>

      {draft.rules.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border px-4 py-8 text-center">
          <p className="text-sm text-muted-foreground">还没有任何规则</p>
          <p className="mt-1 text-xs text-muted-foreground">
            从左边的列表里挑一个应用，点「加规则」
          </p>
        </div>
      ) : (
        <ul className="flex flex-col gap-3">
          <AnimatePresence initial={false}>
            {draft.rules.map((rule, index) => (
              <motion.li
                // Keyed on position only. Keying on the process name as well
                // would give the card a new identity on every keystroke in the
                // process field, remounting the input and taking the caret
                // with it.
                key={index}
                layout={reduce ? false : 'position'}
                initial={reduce ? false : { opacity: 0, y: -8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={reduce ? undefined : { opacity: 0, height: 0 }}
                transition={{ duration: 0.2, ease: [0.16, 1, 0.3, 1] }}
              >
                <RuleCard
                  rule={rule}
                  index={index}
                  duplicate={duplicates.has(normalizeProcess(rule.process))}
                  fields={fields}
                  edit={edit}
                />
              </motion.li>
            ))}
          </AnimatePresence>
        </ul>
      )}
    </section>
  )
}

function RuleCard({
  rule,
  index,
  duplicate,
  fields,
  edit,
}: {
  rule: Rule
  index: number
  duplicate: boolean
  fields: string[]
  edit: (change: (draft: Draft) => Draft) => void
}) {
  const patterns = patternsOf(rule)
  const flagged = (key: string) => fields.includes(`rules[${index}].${key}`)

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto]">
        <Field
          label="进程"
          htmlFor={`rule-${index}-process`}
          error={
            duplicate
              ? '这个进程已经有规则了，同一个进程只能有一条'
              : flagged('process')
                ? '进程名不能为空'
                : undefined
          }
        >
          <Input
            id={`rule-${index}-process`}
            value={rule.process}
            spellCheck={false}
            className="font-mono"
            placeholder="code.exe"
            aria-invalid={duplicate || flagged('process')}
            onChange={(e) => edit((d) => updateRule(d, index, { process: e.target.value }))}
          />
        </Field>

        <Field
          label="显示名"
          htmlFor={`rule-${index}-app`}
          error={flagged('app') ? '显示名不能为空' : undefined}
        >
          <Input
            id={`rule-${index}-app`}
            value={rule.app}
            placeholder="VS Code"
            aria-invalid={flagged('app')}
            onChange={(e) => edit((d) => updateRule(d, index, { app: e.target.value }))}
          />
        </Field>

        <Field label="在干什么" htmlFor={`rule-${index}-description`}>
          <Input
            id={`rule-${index}-description`}
            value={rule.description}
            placeholder="在写代码"
            onChange={(e) =>
              edit((d) => updateRule(d, index, { description: e.target.value }))
            }
          />
        </Field>

        <div className="flex items-end">
          <Button
            variant="ghost"
            size="icon"
            className="text-muted-foreground hover:text-destructive"
            onClick={() => edit((d) => removeRule(d, index))}
          >
            <Trash2 aria-hidden />
            <span className="sr-only">删除 {rule.process || '这条'} 的规则</span>
          </Button>
        </div>
      </div>

      <div className="mt-4 border-t border-border pt-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h3 className="text-xs font-medium">按窗口标题细分</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              从上往下匹配，第一条命中的生效
            </p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() =>
              edit((d) => addPattern(d, index, { match: '', description: '' }))
            }
          >
            <Plus aria-hidden />
            加一条
          </Button>
        </div>

        {patterns.length > 0 ? (
          <ol className="mt-3 flex flex-col gap-2">
            {patterns.map((pattern, patternIndex) => (
              <li key={patternIndex}>
                <PatternRow
                  process={rule.process}
                  match={pattern.match}
                  description={pattern.description}
                  first={patternIndex === 0}
                  last={patternIndex === patterns.length - 1}
                  invalid={fields.includes(
                    `rules[${index}].title_patterns[${patternIndex}].match`,
                  )}
                  onChange={(patch) =>
                    edit((d) => updatePattern(d, index, patternIndex, patch))
                  }
                  onMove={(delta) =>
                    edit((d) => movePattern(d, index, patternIndex, patternIndex + delta))
                  }
                  onRemove={() => edit((d) => removePattern(d, index, patternIndex))}
                />
              </li>
            ))}
          </ol>
        ) : null}
      </div>
    </div>
  )
}

/**
 * One title pattern, with live feedback on what it currently matches.
 *
 * The match count is computed by the agent against the titles it has actually
 * seen. A regular expression that matches nothing is the single most common way
 * to write a rule that silently never fires, and this is what makes that
 * visible while writing it rather than days later.
 */
function PatternRow({
  process,
  match,
  description,
  first,
  last,
  invalid,
  onChange,
  onMove,
  onRemove,
}: {
  process: string
  match: string
  description: string
  first: boolean
  last: boolean
  invalid: boolean
  onChange: (patch: { match?: string; description?: string }) => void
  onMove: (delta: number) => void
  onRemove: () => void
}) {
  const test = usePatternTest(match, process)

  return (
    <div className="rounded-md border border-border bg-background p-2">
      <div className="flex items-start gap-2">
        <div className="flex flex-col">
          <Button
            variant="ghost"
            size="icon"
            className="size-6 text-muted-foreground"
            disabled={first}
            onClick={() => onMove(-1)}
          >
            <ArrowUp aria-hidden className="size-3.5" />
            <span className="sr-only">上移，让它更早匹配</span>
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-6 text-muted-foreground"
            disabled={last}
            onClick={() => onMove(1)}
          >
            <ArrowDown aria-hidden className="size-3.5" />
            <span className="sr-only">下移，让它更晚匹配</span>
          </Button>
        </div>

        <div className="grid min-w-0 flex-1 gap-2 sm:grid-cols-2">
          <div className="grid gap-1">
            <Input
              value={match}
              spellCheck={false}
              placeholder="(?i)youtube"
              className={cn('font-mono text-xs', invalid && 'border-destructive')}
              aria-label="匹配窗口标题的正则"
              aria-invalid={invalid || test?.valid === false}
              onChange={(e) => onChange({ match: e.target.value })}
            />
            <MatchHint match={match} test={test} />
          </div>
          <Input
            value={description}
            placeholder="在看视频"
            aria-label="命中时显示的描述"
            onChange={(e) => onChange({ description: e.target.value })}
          />
        </div>

        <Button
          variant="ghost"
          size="icon"
          className="size-7 text-muted-foreground hover:text-destructive"
          onClick={onRemove}
        >
          <Trash2 aria-hidden className="size-3.5" />
          <span className="sr-only">删掉这条细分</span>
        </Button>
      </div>
    </div>
  )
}

function MatchHint({
  match,
  test,
}: {
  match: string
  test: PatternTestState | null
}) {
  if (!match.trim()) {
    return <p className="text-xs text-muted-foreground">空的正则会匹配所有标题</p>
  }
  if (!test) return <p className="text-xs text-muted-foreground">检查中</p>
  if (!test.valid) {
    return (
      <p role="alert" className="text-xs text-destructive break-all">
        写法有问题：{test.error}
      </p>
    )
  }
  if (test.total === 0) {
    return <p className="text-xs text-muted-foreground">还没有这个应用的标题样本</p>
  }
  return (
    <p className="flex items-center gap-2 text-xs">
      <Badge variant={test.matched > 0 ? 'default' : 'outline'}>
        命中 {test.matched}/{test.total}
      </Badge>
      {test.matched === 0 ? (
        <span className="text-muted-foreground">当前样本一条都没命中</span>
      ) : null}
    </p>
  )
}

interface PatternTestState {
  valid: boolean
  error?: string
  matched: number
  total: number
}

/**
 * Asks the agent what a pattern matches, on a debounce so that typing does not
 * produce a request per keystroke.
 *
 * The agent answers with Go's regexp engine, which is the engine that will
 * actually run this pattern. Testing it in the browser with JavaScript's engine
 * would agree most of the time, and the times it did not would be exactly the
 * confusing ones.
 */
function usePatternTest(match: string, process: string): PatternTestState | null {
  const [state, setState] = useState<PatternTestState | null>(null)

  useEffect(() => {
    if (!match.trim()) {
      setState(null)
      return
    }
    let cancelled = false
    const id = setTimeout(async () => {
      try {
        const result = await api.testPattern(match, process)
        if (cancelled) return
        setState({
          valid: result.valid,
          error: result.error,
          matched: result.match_count,
          total: result.titles?.length ?? 0,
        })
      } catch {
        if (!cancelled) setState(null)
      }
    }, 250)

    return () => {
      cancelled = true
      clearTimeout(id)
    }
  }, [match, process])

  return state
}

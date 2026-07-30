import type { Draft, Rule, TitlePattern } from '@/types/api'

/**
 * Pure operations on a draft's rule list.
 *
 * They are here rather than inline in components for one reason: rule ordering
 * carries meaning (the first matching title_pattern wins), and an off-by-one in
 * a move would silently change what gets reported. Pure functions can be tested;
 * a setState buried in an onClick cannot.
 *
 * Every one of these returns a new object. Nothing mutates the draft in place.
 */

/** patternsOf normalizes the null the Go side sends for an empty list. */
export function patternsOf(rule: Rule): TitlePattern[] {
  return rule.title_patterns ?? []
}

/** newRule builds a rule for a process discovered in the catalog. */
export function newRule(process: string, app = '', description = ''): Rule {
  return { process, app, description, title_patterns: [] }
}

export function addRule(draft: Draft, rule: Rule): Draft {
  return { ...draft, rules: [...draft.rules, rule] }
}

export function updateRule(draft: Draft, index: number, patch: Partial<Rule>): Draft {
  return {
    ...draft,
    rules: draft.rules.map((rule, i) => (i === index ? { ...rule, ...patch } : rule)),
  }
}

/**
 * removeRule drops a rule and, with it, any expose_title entry pointing at it.
 *
 * Leaving the entry behind would produce a configuration the agent refuses to
 * start with ("process X has no rule"), which the user did not ask for and
 * would have to debug.
 */
export function removeRule(draft: Draft, index: number): Draft {
  const removed = draft.rules[index]
  return {
    ...draft,
    rules: draft.rules.filter((_, i) => i !== index),
    expose_title: removed
      ? draft.expose_title.filter((p) => !sameProcess(p, removed.process))
      : draft.expose_title,
  }
}

export function addPattern(draft: Draft, ruleIndex: number, pattern: TitlePattern): Draft {
  const rule = draft.rules[ruleIndex]
  if (!rule) return draft
  return updateRule(draft, ruleIndex, { title_patterns: [...patternsOf(rule), pattern] })
}

export function updatePattern(
  draft: Draft,
  ruleIndex: number,
  patternIndex: number,
  patch: Partial<TitlePattern>,
): Draft {
  const rule = draft.rules[ruleIndex]
  if (!rule) return draft
  return updateRule(draft, ruleIndex, {
    title_patterns: patternsOf(rule).map((p, i) =>
      i === patternIndex ? { ...p, ...patch } : p,
    ),
  })
}

export function removePattern(draft: Draft, ruleIndex: number, patternIndex: number): Draft {
  const rule = draft.rules[ruleIndex]
  if (!rule) return draft
  return updateRule(draft, ruleIndex, {
    title_patterns: patternsOf(rule).filter((_, i) => i !== patternIndex),
  })
}

/**
 * movePattern reorders a rule's patterns. Order is the whole point: the agent
 * uses the first pattern that matches, so "在看视频" before a catch-all and
 * after it are different configurations.
 *
 * A move that would fall off either end is a no-op rather than an error, so the
 * caller can wire it to a button without guarding the edges twice.
 */
export function movePattern(
  draft: Draft,
  ruleIndex: number,
  from: number,
  to: number,
): Draft {
  const rule = draft.rules[ruleIndex]
  if (!rule) return draft

  const patterns = patternsOf(rule)
  if (from === to || from < 0 || to < 0 || from >= patterns.length || to >= patterns.length) {
    return draft
  }

  const reordered = [...patterns]
  const [moved] = reordered.splice(from, 1)
  reordered.splice(to, 0, moved)
  return updateRule(draft, ruleIndex, { title_patterns: reordered })
}

/** exposesTitle reports whether a process reports its raw title verbatim. */
export function exposesTitle(draft: Draft, process: string): boolean {
  return draft.expose_title.some((p) => sameProcess(p, process))
}

/**
 * setExposeTitle turns the raw-title opt-out on or off for one process.
 *
 * Turning it on is guarded by a confirmation dialog in the UI; this function is
 * only the state change.
 */
export function setExposeTitle(draft: Draft, process: string, exposed: boolean): Draft {
  const without = draft.expose_title.filter((p) => !sameProcess(p, process))
  return { ...draft, expose_title: exposed ? [...without, process] : without }
}

/**
 * duplicateProcesses lists process names claimed by more than one rule.
 *
 * The agent rejects duplicates outright, but it only reports the first one. The
 * form marks all of them at once so a user fixing three pasted rules does not
 * have to save three times to find all three.
 */
export function duplicateProcesses(draft: Draft): Set<string> {
  const seen = new Set<string>()
  const duplicates = new Set<string>()
  for (const rule of draft.rules) {
    const key = normalizeProcess(rule.process)
    if (!key) continue
    if (seen.has(key)) duplicates.add(key)
    seen.add(key)
  }
  return duplicates
}

/** normalizeProcess matches the agent's own comparison: trimmed, lowercased. */
export function normalizeProcess(process: string): string {
  return process.trim().toLowerCase()
}

function sameProcess(a: string, b: string): boolean {
  return normalizeProcess(a) === normalizeProcess(b)
}

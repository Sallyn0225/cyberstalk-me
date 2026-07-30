import { describe, expect, it } from 'vitest'
import {
  addPattern,
  addRule,
  duplicateProcesses,
  exposesTitle,
  movePattern,
  newRule,
  patternsOf,
  removePattern,
  removeRule,
  setExposeTitle,
  updatePattern,
  updateRule,
} from './rules'
import { emptyDraft } from '@/types/api'
import type { Draft } from '@/types/api'

function draftWith(rules: Draft['rules'], expose: string[] = []): Draft {
  return { ...emptyDraft, rules, expose_title: expose }
}

const chrome = {
  process: 'chrome.exe',
  app: 'Chrome',
  description: '在上网',
  title_patterns: [
    { match: '(?i)youtube', description: '在看视频' },
    { match: '(?i)github', description: '在看代码' },
    { match: '(?i)mail', description: '在收邮件' },
  ],
}

describe('patternsOf', () => {
  it('turns the Go side’s null into an empty list', () => {
    expect(patternsOf({ ...newRule('a.exe'), title_patterns: null })).toEqual([])
  })
})

describe('movePattern', () => {
  it('moves a pattern up, which changes which one wins', () => {
    const draft = movePattern(draftWith([chrome]), 0, 2, 0)
    expect(patternsOf(draft.rules[0]).map((p) => p.match)).toEqual([
      '(?i)mail',
      '(?i)youtube',
      '(?i)github',
    ])
  })

  it('moves a pattern down', () => {
    const draft = movePattern(draftWith([chrome]), 0, 0, 1)
    expect(patternsOf(draft.rules[0]).map((p) => p.match)).toEqual([
      '(?i)github',
      '(?i)youtube',
      '(?i)mail',
    ])
  })

  it('is a no-op at the edges so the caller need not guard twice', () => {
    const draft = draftWith([chrome])
    for (const [from, to] of [
      [0, -1],
      [2, 3],
      [1, 1],
      [-1, 0],
      [9, 0],
    ]) {
      expect(movePattern(draft, 0, from, to)).toBe(draft)
    }
  })

  it('leaves the draft alone for a rule that is not there', () => {
    const draft = draftWith([chrome])
    expect(movePattern(draft, 5, 0, 1)).toBe(draft)
  })

  it('does not mutate the original', () => {
    const draft = draftWith([chrome])
    const before = patternsOf(draft.rules[0]).map((p) => p.match)
    movePattern(draft, 0, 0, 2)
    expect(patternsOf(draft.rules[0]).map((p) => p.match)).toEqual(before)
  })
})

describe('rule editing', () => {
  it('adds and updates a rule', () => {
    let draft = addRule(emptyDraft, newRule('code.exe', 'VS Code', '在写代码'))
    expect(draft.rules).toHaveLength(1)
    draft = updateRule(draft, 0, { app: 'Visual Studio Code' })
    expect(draft.rules[0]).toEqual({
      process: 'code.exe',
      app: 'Visual Studio Code',
      description: '在写代码',
      title_patterns: [],
    })
  })

  it('adds, updates and removes patterns', () => {
    let draft = addRule(emptyDraft, newRule('chrome.exe', 'Chrome'))
    draft = addPattern(draft, 0, { match: '(?i)youtube', description: '在看视频' })
    draft = addPattern(draft, 0, { match: '(?i)github', description: '' })
    expect(patternsOf(draft.rules[0])).toHaveLength(2)

    draft = updatePattern(draft, 0, 1, { description: '在看代码' })
    expect(patternsOf(draft.rules[0])[1].description).toBe('在看代码')

    draft = removePattern(draft, 0, 0)
    expect(patternsOf(draft.rules[0])).toEqual([
      { match: '(?i)github', description: '在看代码' },
    ])
  })

  it('removes the expose_title entry along with its rule', () => {
    const draft = removeRule(
      draftWith([newRule('code.exe', 'VS Code'), chrome], ['Chrome.EXE', 'code.exe']),
      1,
    )
    expect(draft.rules.map((r) => r.process)).toEqual(['code.exe'])
    // Leaving "Chrome.EXE" behind would produce a config the agent refuses to
    // start with, matched case-insensitively just as the agent matches it.
    expect(draft.expose_title).toEqual(['code.exe'])
  })
})

describe('expose_title', () => {
  it('reports and toggles the opt-out case-insensitively', () => {
    const draft = draftWith([chrome], ['CHROME.EXE'])
    expect(exposesTitle(draft, 'chrome.exe')).toBe(true)
    expect(exposesTitle(draft, 'code.exe')).toBe(false)

    const off = setExposeTitle(draft, 'chrome.exe', false)
    expect(off.expose_title).toEqual([])

    const on = setExposeTitle(off, 'chrome.exe', true)
    expect(on.expose_title).toEqual(['chrome.exe'])
  })

  it('does not list a process twice when turned on again', () => {
    const draft = setExposeTitle(draftWith([chrome], ['chrome.exe']), 'Chrome.exe', true)
    expect(draft.expose_title).toEqual(['Chrome.exe'])
  })
})

describe('duplicateProcesses', () => {
  it('finds duplicates the way the agent compares them', () => {
    const draft = draftWith([
      newRule('code.exe', 'A'),
      newRule('CODE.EXE', 'B'),
      newRule('  code.exe  ', 'C'),
      newRule('chrome.exe', 'D'),
    ])
    expect([...duplicateProcesses(draft)]).toEqual(['code.exe'])
  })

  it('ignores blank process names, which have their own message', () => {
    expect(duplicateProcesses(draftWith([newRule(''), newRule('   ')])).size).toBe(0)
  })

  it('finds nothing when every rule is distinct', () => {
    expect(duplicateProcesses(draftWith([newRule('a.exe'), newRule('b.exe')])).size).toBe(0)
  })
})

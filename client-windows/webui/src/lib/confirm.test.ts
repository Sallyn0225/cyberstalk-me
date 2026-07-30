import { describe, expect, it } from 'vitest'
import { confirmationMatches, confirmationPhrase } from './confirm'

describe('confirmationPhrase', () => {
  it('is the process name, so the user reads which app they are exposing', () => {
    expect(confirmationPhrase('chrome.exe')).toBe('chrome.exe')
    expect(confirmationPhrase('  code.exe  ')).toBe('code.exe')
  })
})

describe('confirmationMatches', () => {
  it('accepts the process name', () => {
    expect(confirmationMatches('chrome.exe', 'chrome.exe')).toBe(true)
  })

  it('forgives case and surrounding whitespace', () => {
    expect(confirmationMatches('  CHROME.exe ', 'chrome.exe')).toBe(true)
  })

  it('rejects anything else, including a near miss', () => {
    for (const typed of ['', ' ', 'chrome', 'chrome.ex', 'chromeexe', 'code.exe', '确认']) {
      expect(confirmationMatches(typed, 'chrome.exe')).toBe(false)
    }
  })

  it('never matches when there is no process to confirm', () => {
    // Otherwise an empty input against an empty process would open the gate.
    expect(confirmationMatches('', '')).toBe(false)
    expect(confirmationMatches('anything', '   ')).toBe(false)
  })
})

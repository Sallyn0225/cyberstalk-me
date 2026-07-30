/**
 * Mirrors the Go types in client-windows/internal/setup.
 *
 * Every field name here is spelled the way it is spelled in config.yaml, which
 * is also how the Go side spells it on the wire. When a key changes, it changes
 * in config.Config, setup.Draft and this file together.
 */

/** mapping.TitlePattern */
export interface TitlePattern {
  match: string
  description: string
}

/** mapping.Rule */
export interface Rule {
  process: string
  app: string
  description: string
  title_patterns: TitlePattern[] | null
}

/** setup.Draft */
export interface Draft {
  server_url: string
  device_id: string
  token: string
  interval: string
  device_name: string
  idle_threshold: string
  default_app: string
  default_description: string
  locked_app: string
  locked_description: string
  rules: Rule[]
  expose_title: string[]
}

/** setup.ValidationIssue */
export interface ValidationIssue {
  message: string
  /** YAML paths, e.g. "rules[1].title_patterns[0].match". */
  fields?: string[]
}

/** setup.State */
export interface State {
  draft: Draft
  config_path: string
  backup_path: string
  notice?: string
  defaults: Draft
  valid: boolean
  error?: ValidationIssue
}

/** setup.TitleSample */
export interface TitleSample {
  title: string
  first_seen: string
  last_seen: string
  count: number
}

/** setup.App */
export interface CatalogApp {
  process: string
  first_seen: string
  last_seen: string
  count: number
  samples: TitleSample[] | null
  configurable: boolean
}

/** setup.CatalogSnapshot */
export interface CatalogSnapshot {
  apps: CatalogApp[] | null
  locked: boolean
  locked_seen: number
  observations: number
}

/** shared.Activity */
export interface Activity {
  app: string
  description: string
  idle: boolean
  idle_seconds: number
  locked: boolean
}

/** setup.Preview */
export interface Preview {
  activity: Activity
  process: string
  configurable: boolean
}

/** setup.PatternTest + setup.RegexTestResult */
export interface RegexTestResult {
  valid: boolean
  error?: string
  matched: boolean[]
  match_count: number
  titles: string[] | null
}

/** setup.SaveResult */
export interface SaveResult {
  saved: boolean
  config_path: string
  backup_path?: string
  error?: ValidationIssue
}

/** An empty draft, for the brief moment before the first load resolves. */
export const emptyDraft: Draft = {
  server_url: '',
  device_id: '',
  token: '',
  interval: '',
  device_name: '',
  idle_threshold: '',
  default_app: '',
  default_description: '',
  locked_app: '',
  locked_description: '',
  rules: [],
  expose_title: [],
}

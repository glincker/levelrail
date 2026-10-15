export type LogRuleKind = 'multiline' | 'redact' | 'parse' | 'drop'

export interface LogRule {
  kind: LogRuleKind
  pattern: string
  replace?: string
}

export interface LogPipeline {
  rules: LogRule[]
}

export interface PreviewLine {
  message: string
  dropped: boolean
  fields?: Record<string, string>
}

export interface PreviewResult {
  lines: PreviewLine[]
}

export const LOG_RULE_KINDS: LogRuleKind[] = [
  'multiline',
  'redact',
  'parse',
  'drop',
]
export const MAX_LOG_RULES = 20

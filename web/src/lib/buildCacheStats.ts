import type { LogLine } from '../hooks/useLogStream'
import { stripAnsiCodes } from './ansi'

export interface BuildCacheStats {
  cached: number
  total: number
}

// BuildKit plain progress prefixes each step with "#N " and prints
// "#N CACHED" when the step was reused. Steps are counted by their
// "[stage i/n]" header so internal vertices (context, metadata) are skipped.
const STEP_HEADER = /^#(\d+)\s+\[[^\]]*\d+\/\d+\]/
const CACHED = /^#(\d+)\s+CACHED\b/

// parseBuildCacheStats returns null when the log has no BuildKit steps, so
// callers can show "unknown" rather than a misleading 0 of 0.
export function parseBuildCacheStats(lines: LogLine[]): BuildCacheStats | null {
  const steps = new Set<string>()
  const cached = new Set<string>()
  for (const l of lines) {
    const text = stripAnsiCodes(l.line).trim()
    const header = STEP_HEADER.exec(text)
    if (header?.[1]) {
      steps.add(header[1])
      continue
    }
    const hit = CACHED.exec(text)
    if (hit?.[1]) {
      cached.add(hit[1])
    }
  }
  if (steps.size === 0) {
    return null
  }
  let reused = 0
  for (const id of cached) {
    if (steps.has(id)) reused += 1
  }
  return { cached: reused, total: steps.size }
}

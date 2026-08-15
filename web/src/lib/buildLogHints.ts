import type { LogLine } from '../hooks/useLogStream'

export interface BuildLogHintMatch {
  id: string
  title: string
  hint: string
  lineId: number
  lineText: string
}

interface BuildLogHintRule {
  id: string
  pattern: RegExp
  title: string
  hint: string
}

// Signatures chosen against what internal/build's relayProgress actually
// relays to the client: only raw RUN-step stdout/stderr (BuildKit
// VertexLog) and the docker-image-load phase's stream text ever become a
// LogLine here. Vertex-level failures (a denied registry pull, a missing
// Dockerfile, an oversized build context) surface only as
// DeployAttempt.Error, never as a log line, so patterns for those would
// never match anything in this view and are deliberately left out.
const RULES: BuildLogHintRule[] = [
  {
    id: 'npm-registry-auth',
    pattern: /npm ERR!\s*(code E401|401 Unauthorized|code E403|403 Forbidden)/i,
    title: 'Package registry rejected credentials',
    hint: 'Check the registry auth env vars (e.g. an npm token secret) declared in app.yaml.',
  },
  {
    id: 'npm-dependency-not-found',
    pattern: /npm ERR!\s*(code E404|404 Not Found)/i,
    title: 'A dependency could not be resolved',
    hint: 'Confirm the package name/version in package.json and that any private registry is reachable.',
  },
  {
    id: 'pip-dependency-not-found',
    pattern: /Could not find a version that satisfies the requirement/i,
    title: 'A Python dependency could not be resolved',
    hint: 'Confirm the package name/version pin and that the configured index URL is reachable.',
  },
  {
    id: 'node-heap-oom',
    pattern: /JavaScript heap out of memory/i,
    title: 'Build ran out of memory',
    hint: 'The build process exhausted available memory. Increase the build node memory or trim a memory-heavy step.',
  },
  {
    id: 'go-runtime-oom',
    pattern: /fatal error: runtime: out of memory/i,
    title: 'Build ran out of memory',
    hint: 'The Go build process exhausted available memory. Increase the build node memory.',
  },
  {
    id: 'disk-space',
    pattern: /no space left on device/i,
    title: 'Build node ran out of disk space',
    hint: 'Free disk space on the build node, or reduce build cache/context size.',
  },
  {
    id: 'permission-denied',
    pattern: /permission denied/i,
    title: 'A build step hit a permission error',
    hint: 'Check file ownership/permissions and any USER instruction in the Dockerfile.',
  },
]

// matchBuildLogHints scans lines once and returns at most one match per
// rule (its first occurrence), newest-seen-pattern-first is not
// preserved; callers get insertion order matching RULES, so the list is
// stable across renders regardless of scan order.
export function matchBuildLogHints(lines: LogLine[]): BuildLogHintMatch[] {
  const firstMatch = new Map<string, LogLine>()
  for (const rule of RULES) {
    const found = lines.find((line) => rule.pattern.test(line.line))
    if (found) {
      firstMatch.set(rule.id, found)
    }
  }

  const matches: BuildLogHintMatch[] = []
  for (const rule of RULES) {
    const line = firstMatch.get(rule.id)
    if (!line) {
      continue
    }
    matches.push({
      id: rule.id,
      title: rule.title,
      hint: rule.hint,
      lineId: line.id,
      lineText: line.line,
    })
  }
  return matches
}

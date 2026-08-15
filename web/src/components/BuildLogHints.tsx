import { useState } from 'react'
import { LightbulbIcon } from '@phosphor-icons/react/dist/ssr'
import type { LogLine } from '../hooks/useLogStream'
import {
  buildLogHintMatches,
  initialBuildLogHintScanState,
  scanBuildLogHints,
  type BuildLogHintScanState,
} from '../lib/buildLogHints'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'

// Purely cosmetic, heuristic annotations over the raw log lines
// LogTerminal already renders: a substring match can be a false
// positive (e.g. an app printing "permission denied" about its own
// unrelated file), so this never touches the deploy attempt's real
// status. It only ever adds a maybe-wrong suggestion below already
// visible logs, never gates or relabels the pass/fail outcome.
export function BuildLogHints({ lines }: { lines: LogLine[] }) {
  // "Adjusting state during render" (React-sanctioned pattern for
  // deriving state from a prop change): scanBuildLogHints only rescans
  // lines newer than the last render, so a long build log doesn't get
  // rescanned in full on every new line. A ref would do the same job but
  // reading ref.current during render is disallowed (react-hooks/refs).
  const [scan, setScan] = useState<{
    lines: LogLine[]
    state: BuildLogHintScanState
  }>(() => ({ lines, state: scanBuildLogHints(lines, initialBuildLogHintScanState) }))

  const state =
    scan.lines === lines ? scan.state : scanBuildLogHints(lines, scan.state)
  if (state !== scan.state) {
    setScan({ lines, state })
  }

  const matches = buildLogHintMatches(state)
  if (matches.length === 0) {
    return null
  }

  return (
    <div className="space-y-2">
      {matches.map((m) => (
        <Alert key={m.id}>
          <LightbulbIcon />
          <AlertTitle>{m.title}</AlertTitle>
          <AlertDescription>{m.hint}</AlertDescription>
        </Alert>
      ))}
    </div>
  )
}

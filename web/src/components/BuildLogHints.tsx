import { LightbulbIcon } from '@phosphor-icons/react/dist/ssr'
import type { LogLine } from '../hooks/useLogStream'
import { matchBuildLogHints } from '../lib/buildLogHints'
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert'

// Purely cosmetic, heuristic annotations over the raw log lines
// LogTerminal already renders: a substring match can be a false
// positive (e.g. an app printing "permission denied" about its own
// unrelated file), so this never touches the deploy attempt's real
// status. It only ever adds a maybe-wrong suggestion below already
// visible logs, never gates or relabels the pass/fail outcome.
export function BuildLogHints({ lines }: { lines: LogLine[] }) {
  const matches = matchBuildLogHints(lines)
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

import { diffEnv } from '../lib/envParse'
import { Badge } from '@/components/ui/badge'

const SIGN_CLASS = {
  '+': 'text-glinr-code-added',
  '~': 'text-glinr-code-log-warn',
  '-': 'text-glinr-code-removed',
} as const

function KeyList({
  label,
  keys,
  sign,
}: {
  label: string
  keys: string[]
  sign: keyof typeof SIGN_CLASS
}) {
  if (keys.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Badge variant="outline">
        {label} {keys.length}
      </Badge>
      {keys.map((k) => (
        <span key={k} className="font-mono text-xs">
          <span
            aria-hidden="true"
            className={`mr-1 font-semibold select-none ${SIGN_CLASS[sign]}`}
          >
            {sign}
          </span>
          {k}
        </span>
      ))}
    </div>
  )
}

// Summary of what Save variables would change versus the saved values.
// Renders nothing when the form matches what is saved.
export function EnvPendingDiff({
  saved,
  vars,
}: {
  saved: Record<string, string>
  vars: { key: string; value: string }[]
}) {
  const pending: Record<string, string> = {}
  for (const v of vars) {
    const key = v.key.trim()
    if (key) pending[key] = v.value
  }
  const { added, changed, removed } = diffEnv(saved, pending)
  if (added.length + changed.length + removed.length === 0) return null

  return (
    <div
      className="space-y-1.5 rounded-md border border-border bg-muted/40 p-3"
      data-testid="env-pending-diff"
    >
      <p className="text-xs font-medium text-muted-foreground">
        Unsaved changes
      </p>
      <KeyList label="Added" keys={added} sign="+" />
      <KeyList label="Changed" keys={changed} sign="~" />
      <KeyList label="Removed" keys={removed} sign="-" />
    </div>
  )
}

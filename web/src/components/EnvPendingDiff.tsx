import { diffEnv } from '../lib/envParse'
import { Badge } from '@/components/ui/badge'

function KeyList({ label, keys }: { label: string; keys: string[] }) {
  if (keys.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Badge variant="outline">
        {label} {keys.length}
      </Badge>
      {keys.map((k) => (
        <span key={k} className="font-mono text-xs">
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
      <KeyList label="Added" keys={added} />
      <KeyList label="Changed" keys={changed} />
      <KeyList label="Removed" keys={removed} />
    </div>
  )
}

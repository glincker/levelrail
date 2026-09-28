import { StatusPill } from '@/components/kit'
import { moveSteps } from '../lib/moveSteps'

export function MovePlan({
  appName,
  volumeNames,
}: {
  appName: string
  volumeNames: string[]
}) {
  return (
    <div className="space-y-2 rounded-md border border-border p-3 text-xs">
      <div className="flex items-center gap-2">
        <p className="font-medium text-foreground">Plan</p>
        <StatusPill tone="warning" label="Downtime" size="sm" />
      </div>
      <ol className="list-decimal space-y-0.5 pl-4 text-muted-foreground">
        {moveSteps(appName, volumeNames).map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ol>
      <p className="text-muted-foreground">
        Downtime lasts until every volume has copied, so it grows with volume
        size. Source volumes are never deleted.
      </p>
      <p className="text-muted-foreground">
        There is no automatic health gate or rollback: if a step fails the app
        stays stopped, the failing step is shown here, and you can retry the
        move safely.
      </p>
    </div>
  )
}

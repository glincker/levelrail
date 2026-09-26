import {
  PlayIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { formatAge } from '../lib/format'
import {
  useControlPlaneDr,
  useRunControlPlaneDrBackup,
  useRunControlPlaneDrDrill,
  type ControlPlaneDrDrill,
} from '../queries/controlPlaneDr'
import { ControlPlaneDrChecklistView } from './ControlPlaneDrChecklist'
import { ControlPlaneDrEscrow } from './ControlPlaneDrEscrow'
import { ControlPlaneDrSettings } from './ControlPlaneDrSettings'

function drillSummary(d: ControlPlaneDrDrill): string {
  if (d.at === undefined) return 'never run'
  const when = formatAge(d.at)
  if (!d.ok) return `failed ${when}: ${d.detail}`
  return d.partial
    ? `passed ${when} (partial: checksum only, decryption untested)`
    : `passed ${when} in ${Math.max(1, Math.round(d.duration_ms / 1000))}s`
}

export function ControlPlaneDrCard() {
  const { data: status, isLoading, isError } = useControlPlaneDr()
  const run = useRunControlPlaneDrBackup()
  const drill = useRunControlPlaneDrDrill()

  // Returns 501 when there is no master key and 403 for a non-root session.
  if (isLoading || isError || !status) {
    return null
  }

  const start = (mutate: typeof run.mutate, label: string) => {
    mutate(undefined, {
      onSuccess: () => {
        toast.add({ title: `${label} started.`, type: 'success' })
      },
      onError: (e) => {
        toast.add({ title: e.message, type: 'error' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <ShieldCheckIcon className="size-4" />
            </div>
            <div>
              <CardTitle>Disaster recovery</CardTitle>
              <CardDescription>
                Encrypted off-box backups, key escrow and restore drills.
              </CardDescription>
            </div>
          </div>
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={
                run.isPending || status.backup_running || !status.configured
              }
              onClick={() => {
                start(run.mutate, 'Backup')
              }}
            >
              <PlayIcon className="size-3.5" aria-hidden="true" />
              {status.backup_running ? 'Backing up...' : 'Run now'}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={
                drill.isPending || status.drill_running || !status.configured
              }
              onClick={() => {
                start(drill.mutate, 'Restore drill')
              }}
            >
              {status.drill_running ? 'Running drill...' : 'Run drill'}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {status.warnings.length > 0 ? (
          <ul className="space-y-1.5" aria-label="Warnings">
            {status.warnings.map((w) => (
              <li
                key={w.code}
                className="flex items-start gap-2 rounded-md border border-border bg-muted/40 p-2.5 text-sm text-foreground"
              >
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0 text-amber-600"
                  aria-hidden="true"
                />
                <span>{w.message}</span>
              </li>
            ))}
          </ul>
        ) : null}

        <ControlPlaneDrChecklistView checklist={status.checklist} />

        <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
          <dt className="text-muted-foreground">Last backup</dt>
          <dd>
            {status.last_backup_at !== undefined
              ? formatAge(status.last_backup_at)
              : 'never'}
            {status.next_backup_at !== undefined
              ? `, next ${new Date(status.next_backup_at).toLocaleString()}`
              : ''}
          </dd>
          <dt className="text-muted-foreground">Last restore drill</dt>
          <dd>{drillSummary(status.last_drill)}</dd>
          <dt className="text-muted-foreground">Drill identity</dt>
          <dd>
            {status.drill_identity_configured
              ? 'configured, drills decrypt and restore fully'
              : 'not configured, drills verify checksums only (APP_CONTROL_PLANE_DRILL_IDENTITY_FILE)'}
          </dd>
        </dl>

        <ControlPlaneDrEscrow status={status} />

        <div className="border-t border-border pt-4">
          <ControlPlaneDrSettings
            key={`${status.target_id}|${status.recipients.join(',')}|${status.enabled}`}
            status={status}
          />
        </div>
      </CardContent>
    </Card>
  )
}

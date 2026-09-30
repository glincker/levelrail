import { createFileRoute } from '@tanstack/react-router'
import { DatabaseIcon } from '@phosphor-icons/react/dist/ssr'
import { ControlPlaneBackupsCard } from '../../components/ControlPlaneBackupsCard'
import { ControlPlaneDrCard } from '../../components/ControlPlaneDrCard'
import { HelpLink } from '@/components/HelpLink'

// This instance's own database, distinct from routes/backups/index.tsx's
// history of the databases and volumes it manages for deployed apps. Its
// own settings page rather than a card on General: backup and disaster
// recovery are one concern big enough for docs/control-plane-backup.md
// and docs/disaster-recovery.md to each cover on their own.
export const Route = createFileRoute('/settings/control-plane-backup')({
  component: ControlPlaneBackupPage,
})

function ControlPlaneBackupPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <DatabaseIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Control plane backup
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Snapshots of this instance&apos;s own database, off-box encrypted
              backups, key escrow, and restore drills.
            </p>
          </div>
        </div>
        <HelpLink path="/disaster-recovery" label="Disaster recovery guide" />
      </div>
      <ControlPlaneBackupsCard />
      <ControlPlaneDrCard />
    </div>
  )
}

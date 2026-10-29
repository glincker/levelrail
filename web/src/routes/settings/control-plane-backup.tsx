import { createFileRoute } from '@tanstack/react-router'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { ControlPlaneBackupsCard } from '../../components/ControlPlaneBackupsCard'
import { ControlPlaneDrCard } from '../../components/ControlPlaneDrCard'
import { ArchivesCard } from '../../components/ArchivesCard'
import { PageHeader } from '@/components/shell/PageHeader'

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
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <DatabaseGlassIcon className="size-5" />
        </div>
        <PageHeader
          title="Control plane backup"
          description="Snapshots of this instance's own database, off-box encrypted backups, key escrow, and restore drills."
          helpPath="/disaster-recovery"
          helpLabel="Disaster recovery guide"
        />
      </div>
      <ControlPlaneBackupsCard />
      <ControlPlaneDrCard />
      <ArchivesCard />
    </div>
  )
}

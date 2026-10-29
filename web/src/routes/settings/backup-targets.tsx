import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { backupTargetListQueryOptions } from '../../queries/backupTargets'
import { BackupTargetTable } from '../../components/BackupTargetTable'
import { BackupCopiesSection } from '../../components/BackupCopiesSection'
import { CreateBackupTargetDialog } from '../../components/CreateBackupTargetDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { SetupStepHint } from '@/components/getStarted/SetupStepHint'
import { BackupTargetsRail } from '../../components/settings/SettingsRails'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

// Account-level: a backup target can back up more than one database, so
// it lives here. Triggering a backup and its history live on the owning
// database's page (components/BackupsSection.tsx), not this one.
export const Route = createFileRoute('/settings/backup-targets')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(backupTargetListQueryOptions()),
  component: BackupTargetsPage,
  pendingComponent: BackupTargetsPending,
})

function BackupTargetsPage() {
  const { t } = useTranslation('settings')
  const { data: targets } = useSuspenseQuery(backupTargetListQueryOptions())

  return (
    <PageTemplate
      icon={<CloudArrowUpIcon />}
      eyebrow={t('backupTargetsPage.eyebrow')}
      title={t('backupTargetsPage.title')}
      description={t('backupTargetsPage.description')}
      helpPath="/backups-and-storage"
      helpLabel={t('backupTargetsPage.help')}
      status={
        <Badge variant={targets.length > 0 ? 'success' : 'warning'}>
          {t('backupTargetsPage.count', { count: targets.length })}
        </Badge>
      }
      actions={<CreateBackupTargetDialog />}
      aside={<BackupTargetsRail targets={targets} />}
      asideLabel={t('backupTargetsPage.rail.label')}
      banners={
        targets.length === 0 ? (
          <Banner variant="warning" title={t('backupTargetsPage.banner.title')}>
            {t('backupTargetsPage.banner.body')}
          </Banner>
        ) : null
      }
    >
      <SetupStepHint step="backup_target" />
      <BackupTargetTable
        targets={targets}
        action={<CreateBackupTargetDialog />}
      />
      {targets.length > 1 ? <BackupCopiesSection targets={targets} /> : null}
    </PageTemplate>
  )
}

// Matches BackupTargetTable's 5-column shape so the skeleton doesn't
// jump when real rows swap in.
function BackupTargetsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="backup-targets" />
      <TableSkeleton columnCount={5} />
    </div>
  )
}

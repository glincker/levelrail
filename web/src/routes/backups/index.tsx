import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { useBackupCoverage } from '../../queries/backupCoverage'
import { CoverageList } from '../../components/backups/CoverageList'
import { CoverageSummary } from '../../components/backups/CoverageSummary'
import { DrillTimeline } from '../../components/backups/DrillTimeline'
import { FirstRunSetup } from '../../components/backups/FirstRunSetup'
import { OrphanedDataPanel } from '../../components/backups/OrphanedDataPanel'
import { ProtectAllDialog } from '../../components/backups/ProtectAllDialog'
import { RecentActivity } from '../../components/backups/RecentActivity'
import { StorageSafetyCard } from '../../components/backups/StorageSafetyCard'

export const Route = createFileRoute('/backups/')({
  component: BackupsPage,
})

/** shouldPromptForBackupTarget is true only once targets loaded and none exist. */
export function shouldPromptForBackupTarget(
  targetsLoading: boolean,
  targetCount: number,
): boolean {
  return !targetsLoading && targetCount === 0
}

function BackupsPage() {
  const { t } = useTranslation('backups')
  const coverage = useBackupCoverage()
  const [protectOpen, setProtectOpen] = useState(false)
  const data = coverage.data

  return (
    <PageTemplate
      title={t('page.title')}
      description={t('page.description')}
      icon={<CloudArrowUpIcon />}
      helpPath="/backups-and-storage"
      helpLabel={t('page.helpLabel')}
    >
      {coverage.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t('page.loadFailed', { error: coverage.error.message })}
        </p>
      ) : !data ? (
        <div className="space-y-3" aria-busy="true">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : data.targets.length === 0 ? (
        <FirstRunSetup unprotected={data.summary.total} />
      ) : (
        <>
          <CoverageSummary
            summary={data.summary}
            onProtect={() => {
              setProtectOpen(true)
            }}
          />
          {data.resources.length === 0 ? (
            <EmptyState
              icon={<CloudArrowUpIcon className="size-5" />}
              title={t('page.noResourcesTitle')}
              description={t('page.noResourcesDescription')}
            />
          ) : (
            <CoverageList
              rows={data.resources}
              targets={data.targets}
              onNeedTarget={() => {
                setProtectOpen(true)
              }}
            />
          )}
          <div className="grid gap-4 md:grid-cols-2">
            <StorageSafetyCard encryption={data.encryption} />
            <DrillTimeline />
          </div>
        </>
      )}

      <OrphanedDataPanel />
      {data && data.targets.length > 0 ? <RecentActivity /> : null}

      {data ? (
        <ProtectAllDialog
          open={protectOpen}
          onOpenChange={setProtectOpen}
          targets={data.targets}
          defaults={data.defaults}
          scratchCount={data.summary.scratch}
        />
      ) : null}
    </PageTemplate>
  )
}

import {
  createFileRoute,
  type ErrorComponentProps,
} from '@tanstack/react-router'
import { DemoInstallState } from '@/components/demo/DemoInstallState'
import { isDemoFinalError } from '@/lib/apiError'
import { useTranslation } from 'react-i18next'
import { useSuspenseQuery } from '@tanstack/react-query'
import { ArrowUpGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert'
import { Button } from '../../components/ui/button'
import {
  updatesQueryOptions,
  updateSettingsQueryOptions,
} from '../../queries/updates'
import { policyQueryOptions } from '../../queries/upgradeSchedule'
import { UpgradePreflight } from '../../components/settings/UpgradePreflight'
import { UpgradeAttempts } from '../../components/settings/UpgradeAttempts'
import { RestorePoints } from '../../components/settings/RestorePoints'
import { ServerReadiness } from '../../components/settings/ServerReadiness'
import { ReleaseHistory } from '../../components/settings/ReleaseHistory'
import { UpgradeHistory } from '../../components/settings/UpgradeHistory'
import { PostUpgradeVerify } from '../../components/settings/PostUpgradeVerify'
import { UpdateStatusHeader } from '../../components/settings/updates/UpdateStatusHeader'
import { AutoUpdatesCard } from '../../components/settings/updates/AutoUpdatesCard'
import { SkippedNotes } from '../../components/settings/updates/SkippedNotes'
import { LazySection } from '../../components/settings/updates/LazySection'
import {
  ATTEMPTS_SECTION_ID,
  ROLLBACK_SECTION_ID,
  UPDATE_STATUS_ID,
} from '../../lib/updatesAnchors'
import { PageTemplate } from '@/components/shell/PageTemplate'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

export const Route = createFileRoute('/settings/updates')({
  loader: ({ context: { queryClient } }) => {
    // Not awaited: the card has its own loading and error states.
    void queryClient.prefetchQuery(policyQueryOptions())
    return Promise.all([
      queryClient.ensureQueryData(updatesQueryOptions()),
      queryClient.ensureQueryData(updateSettingsQueryOptions()),
    ])
  },
  component: UpdatesSettingsPage,
  pendingComponent: UpdatesSettingsSkeleton,
  errorComponent: UpdatesSettingsError,
})

function UpdatesSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={1} />
      <SettingsCardSkeleton rows={2} rowVariant="status" />
    </div>
  )
}

function UpdatesSettingsError({ error, reset }: ErrorComponentProps) {
  const { t } = useTranslation('updates')
  if (isDemoFinalError(error)) return <DemoInstallState />
  return (
    <PageTemplate
      icon={<ArrowUpGlassIcon size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
    >
      <Alert variant="destructive">
        <AlertTitle>{t('page.errorTitle')}</AlertTitle>
        <AlertDescription>
          <p>
            {t('page.errorBody', {
              message: error instanceof Error ? error.message : String(error),
            })}
          </p>
          <Button
            type="button"
            variant="outline"
            className="mt-3"
            onClick={reset}
          >
            {t('page.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    </PageTemplate>
  )
}

function UpdatesSettingsPage() {
  const { data: status } = useSuspenseQuery(updatesQueryOptions())
  const { t } = useTranslation('updates')

  return (
    <PageTemplate
      icon={<ArrowUpGlassIcon size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
    >
      <AutoUpdatesCard />
      <div id={UPDATE_STATUS_ID}>
        <UpdateStatusHeader initial={status} />
      </div>
      <div className="divide-y divide-border rounded-lg border border-border bg-card">
        <LazySection id={ATTEMPTS_SECTION_ID} title={t('quick.more.history')}>
          <UpgradeAttempts currentVersion={status.current_version} />
          <UpgradeHistory />
          <PostUpgradeVerify />
        </LazySection>
        <LazySection title={t('skipped.title')}>
          <SkippedNotes />
        </LazySection>
        <LazySection title={t('quick.more.checks')}>
          <UpgradePreflight hideEdgeGuidance />
          <ServerReadiness />
        </LazySection>
        <LazySection id={ROLLBACK_SECTION_ID} title={t('quick.more.restore')}>
          <ReleaseHistory />
          <RestorePoints />
        </LazySection>
      </div>
    </PageTemplate>
  )
}

import {
  createFileRoute,
  type ErrorComponentProps,
} from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useSuspenseQuery } from '@tanstack/react-query'
import { ArrowUpGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '../../components/ui/card'
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert'
import { Button } from '../../components/ui/button'
import {
  updatesQueryOptions,
  updateSettingsQueryOptions,
} from '../../queries/updates'
import { UpgradePreflight } from '../../components/settings/UpgradePreflight'
import { UpgradeAttempts } from '../../components/settings/UpgradeAttempts'
import { RestorePoints } from '../../components/settings/RestorePoints'
import { AutomaticUpgrades } from '../../components/settings/AutomaticUpgrades'
import { ServerReadiness } from '../../components/settings/ServerReadiness'
import { ReleaseHistory } from '../../components/settings/ReleaseHistory'
import { UpgradeHistory } from '../../components/settings/UpgradeHistory'
import { PostUpgradeVerify } from '../../components/settings/PostUpgradeVerify'
import { UpdateStatusHeader } from '../../components/settings/updates/UpdateStatusHeader'
import { UpdateSettingsForm } from '../../components/settings/updates/UpdateSettingsForm'
import { ROLLBACK_SECTION_ID } from '../../lib/updatesAnchors'
import { PageTemplate } from '@/components/shell/PageTemplate'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

export const Route = createFileRoute('/settings/updates')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(updatesQueryOptions()),
      queryClient.ensureQueryData(updateSettingsQueryOptions()),
    ]),
  component: UpdatesSettingsPage,
  pendingComponent: UpdatesSettingsSkeleton,
  errorComponent: UpdatesSettingsError,
})

function UpdatesSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={3} rowVariant="status" />
      <div className="grid gap-4 lg:grid-cols-2">
        <SettingsCardSkeleton rows={2} />
        <SettingsCardSkeleton rows={2} />
      </div>
      <SettingsCardSkeleton rows={3} rowVariant="list" />
    </div>
  )
}

function UpdatesSettingsError({ error, reset }: ErrorComponentProps) {
  const { t } = useTranslation('updates')
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

function SectionCard({
  title,
  description,
  className,
  id,
  children,
}: {
  title: string
  description: string
  className?: string
  id?: string
  children: React.ReactNode
}) {
  return (
    <Card id={id} className={className}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription className="max-w-prose">{description}</CardDescription>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  )
}

function UpdatesSettingsPage() {
  const { data: status } = useSuspenseQuery(updatesQueryOptions())
  const { data: settings } = useSuspenseQuery(updateSettingsQueryOptions())
  const { t } = useTranslation('updates')

  return (
    <PageTemplate
      icon={<ArrowUpGlassIcon size="1.4em" />}
      title={t('page.title')}
      description={t('page.description')}
    >
      <UpdateStatusHeader initial={status} />
      <UpdateSettingsForm settings={settings} />
      <SectionCard title={t('auto.title')} description={t('auto.description')}>
        <AutomaticUpgrades />
      </SectionCard>
      <div className="grid items-start gap-4 lg:grid-cols-2">
        <SectionCard
          title={t('preflight.cardTitle')}
          description={t('preflight.cardDescription')}
        >
          <UpgradePreflight hideEdgeGuidance />
        </SectionCard>
        <SectionCard
          title={t('readiness.title')}
          description={t('readiness.description')}
        >
          <ServerReadiness />
        </SectionCard>
      </div>
      <SectionCard
        title={t('selfUpgrade.attemptsTitle')}
        description={t('selfUpgrade.description')}
      >
        <UpgradeAttempts currentVersion={status.current_version} />
      </SectionCard>
      <SectionCard
        id={ROLLBACK_SECTION_ID}
        title={t('history.title')}
        description={t('history.description')}
      >
        <ReleaseHistory />
        <RestorePoints />
      </SectionCard>
      <SectionCard
        title={t('upgradeHistory.title')}
        description={t('upgradeHistory.description')}
      >
        <UpgradeHistory />
      </SectionCard>
      <SectionCard
        title={t('verify.title')}
        description={t('verify.description')}
      >
        <PostUpgradeVerify />
      </SectionCard>
    </PageTemplate>
  )
}

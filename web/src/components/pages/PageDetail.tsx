import { Suspense, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutTemplateGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DeployAttemptsList } from '../DeployAttemptsList'
import { DomainEditor } from '../DomainEditor'
import { PageDropTarget } from './PageDropTarget'
import { RollbackCard } from './RollbackCard'
import { SiteHeaderMeta } from './SiteHeaderMeta'
import { PageDomains } from './PageDomains'
import { SitePublisher } from '../dropdeploy/SitePublisher'
import { StaticRoutesPanel } from '../static/StaticRoutesPanel'
import { useDeployProgress } from '../../hooks/useDeployProgress'
import { PAGE_TABS, type PageTab } from './pageHelpers'
import { useApp, useDeleteApp } from '../../queries/apps'

function Deployments({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const { attempts, conditions } = useDeployProgress(name)
  const [uploading, setUploading] = useState(false)
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button
          size="sm"
          variant="outline"
          aria-expanded={uploading}
          onClick={() => setUploading((v) => !v)}
        >
          {uploading ? t('detail.hidePublish') : t('detail.publish')}
        </Button>
      </div>
      {uploading ? (
        <div className="rounded-lg border border-border bg-card p-3">
          <SitePublisher appName={name} />
        </div>
      ) : null}
      <RollbackCard name={name} attempts={attempts} />
      <DeployAttemptsList
        appName={name}
        attempts={attempts}
        conditions={conditions}
      />
    </div>
  )
}

function Domains({ name }: Readonly<{ name: string }>) {
  const { data: app } = useApp(name)
  return (
    <div className="space-y-4">
      <PageDomains name={name} />
      <DomainEditor app={app} />
    </div>
  )
}

function Settings({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const navigate = useNavigate()
  const del = useDeleteApp()
  const [confirming, setConfirming] = useState(false)
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3">
        <div>
          <p className="text-sm font-medium">{t('settings.app')}</p>
          <p className="text-xs text-muted-foreground">
            {t('settings.appHint')}
          </p>
        </div>
        <Button
          size="sm"
          variant="outline"
          render={<Link to="/apps/$name" params={{ name }} />}
        >
          {t('settings.openApp')}
        </Button>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3">
        <div>
          <p className="text-sm font-medium">{t('settings.danger')}</p>
          <p className="text-xs text-muted-foreground">
            {t('settings.dangerHint')}
          </p>
        </div>
        <Button
          size="sm"
          variant="destructive"
          onClick={() => setConfirming(true)}
        >
          {t('settings.delete')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('settings.confirmTitle', { name })}
        description={t('settings.confirmBody')}
        confirmLabel={t('settings.delete')}
        pendingLabel={t('settings.deleting')}
        tone="destructive"
        requireTyped={name}
        pending={del.isPending}
        error={del.error?.message ?? null}
        onConfirm={() =>
          del.mutate(name, {
            onSuccess: () => void navigate({ to: '/pages' }),
          })
        }
      />
    </div>
  )
}

export function PageDetail({
  name,
  tab,
  onTabChange,
}: Readonly<{
  name: string
  tab: PageTab
  onTabChange: (tab: PageTab) => void
}>) {
  const { t } = useTranslation('pagesSites')
  const { data: app } = useApp(name)
  const domain = app.domains?.[0]
  const fallback = <Skeleton className="h-24 w-full" />
  return (
    <PageTemplate
      icon={<LayoutTemplateGlassIcon size="1.4em" />}
      title={name}
      breadcrumb={
        <Link
          to="/pages"
          className="text-xs text-muted-foreground underline-offset-2 hover:underline focus-visible:underline"
        >
          {t('detail.back')}
        </Link>
      }
      width="wide"
      actions={
        domain ? (
          <Button
            size="sm"
            variant="outline"
            render={
              <a href={`https://${domain}`} target="_blank" rel="noreferrer" />
            }
          >
            {t('detail.visit')}
          </Button>
        ) : null
      }
    >
      <PageDropTarget name={name}>
        <SiteHeaderMeta name={name} />
        <Tabs
          value={tab}
          onValueChange={(v) => {
            const next = PAGE_TABS.find((x) => x === v)
            if (next) onTabChange(next)
          }}
        >
          <TabsList aria-label={t('detail.tabs.label')}>
            {PAGE_TABS.map((id) => (
              <TabsTrigger key={id} value={id}>
                {t(`detail.tabs.${id}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="deployments">
            <Suspense fallback={fallback}>
              <Deployments name={name} />
            </Suspense>
          </TabsContent>
          <TabsContent value="domains">
            <Suspense fallback={fallback}>
              <Domains name={name} />
            </Suspense>
          </TabsContent>
          <TabsContent value="routes">
            <Suspense fallback={fallback}>
              <StaticRoutesPanel app={name} />
            </Suspense>
          </TabsContent>
          <TabsContent value="settings">
            <Settings name={name} />
          </TabsContent>
        </Tabs>
      </PageDropTarget>
    </PageTemplate>
  )
}

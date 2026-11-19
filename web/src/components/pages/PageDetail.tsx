import { lazy, Suspense, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LayoutTemplateGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DeployAttemptsList } from '../DeployAttemptsList'
import { PageDropTarget } from './PageDropTarget'
import { RollbackCard } from './RollbackCard'
import { SiteHero } from './SiteHero'
import { PageDomains } from './PageDomains'
import { SitePublisher } from '../dropdeploy/SitePublisher'
import { useDeployProgress } from '../../hooks/useDeployProgress'
import { PAGE_TABS, type PageTab } from './pageHelpers'
import { usePageTabBadges } from './usePageTabBadges'
import { useApp, useDeleteApp } from '../../queries/apps'

// Cards load on demand so the detail route keeps its first-load budget.
const PageOverview = lazy(() => import('./PageOverview'))
const PageRoutes = lazy(() => import('./PageRoutes'))
const SiteSettingsForm = lazy(() => import('./SiteSettingsForm'))
const SiteGitCard = lazy(() => import('./SiteGitCard'))
const SiteFormsCard = lazy(() => import('./SiteFormsCard'))

function Deployments({
  name,
  uploading,
}: Readonly<{ name: string; uploading: boolean }>) {
  const { attempts, conditions } = useDeployProgress(name)
  return (
    <div className="space-y-3">
      {uploading ? (
        <div className="rounded-lg border border-border bg-card p-3">
          <SitePublisher appName={name} />
        </div>
      ) : null}
      <Suspense fallback={null}>
        <SiteGitCard name={name} />
      </Suspense>
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
  return <PageDomains app={app} />
}

function Settings({
  name,
  onDelete,
}: Readonly<{ name: string; onDelete: () => void }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="space-y-2">
      <Suspense fallback={<Skeleton className="h-48 w-full" />}>
        <SiteSettingsForm name={name} />
      </Suspense>
      <Suspense fallback={<Skeleton className="h-16 w-full" />}>
        <SiteFormsCard name={name} />
      </Suspense>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-2.5">
        <p className="text-sm font-medium">{t('settings.app')}</p>
        <Button
          size="sm"
          variant="outline"
          render={<Link to="/apps/$name" params={{ name }} />}
        >
          {t('settings.openApp')}
        </Button>
      </div>
      <Collapsible className="rounded-lg border border-tone-danger/30">
        <CollapsibleTrigger className="px-4 py-2.5 text-sm font-medium text-tone-danger">
          {t('settings.dangerZone')}
        </CollapsibleTrigger>
        <CollapsiblePanel>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-tone-danger/30 px-4 py-2.5">
            <p className="text-sm">{t('settings.dangerHint')}</p>
            <Button size="sm" variant="destructive" onClick={onDelete}>
              {t('settings.delete')}
            </Button>
          </div>
        </CollapsiblePanel>
      </Collapsible>
    </div>
  )
}

export function PageDetail({
  name,
  tab,
  onTabChange,
  redirectFrom,
  onAddRedirect,
}: Readonly<{
  name: string
  tab: PageTab
  onTabChange: (tab: PageTab) => void
  redirectFrom?: string
  onAddRedirect: (path: string) => void
}>) {
  const { t } = useTranslation('pagesSites')
  const navigate = useNavigate()
  const del = useDeleteApp()
  const [confirming, setConfirming] = useState(false)
  const [uploading, setUploading] = useState(false)
  const badges = usePageTabBadges(name)
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
    >
      <PageDropTarget name={name}>
        <SiteHero
          name={name}
          onPublish={() => {
            setUploading(true)
            onTabChange('deployments')
          }}
          onDelete={() => setConfirming(true)}
        />
        <Tabs
          value={tab}
          onValueChange={(v) => {
            const next = PAGE_TABS.find((x) => x === v)
            if (next) onTabChange(next)
          }}
        >
          <div className="sticky top-0 z-10 bg-background py-1">
            <TabsList aria-label={t('detail.tabs.label')}>
              {PAGE_TABS.map((id) => {
                const b = badges[id]
                return (
                  <TabsTrigger key={id} value={id}>
                    {t(`detail.tabs.${id}`)}
                    {b?.count ? (
                      <span className="text-xs text-muted-foreground tabular-nums">
                        {b.count}
                      </span>
                    ) : null}
                    {b?.alert ? (
                      <>
                        <span
                          aria-hidden="true"
                          className="size-1.5 rounded-full bg-destructive"
                        />
                        <span className="sr-only">
                          {t('detail.tabs.failing')}
                        </span>
                      </>
                    ) : null}
                  </TabsTrigger>
                )
              })}
            </TabsList>
          </div>
          <TabsContent value="overview">
            <Suspense fallback={fallback}>
              <PageOverview
                name={name}
                onAddRedirect={onAddRedirect}
                onViewDeploys={() => onTabChange('deployments')}
              />
            </Suspense>
          </TabsContent>
          <TabsContent value="deployments">
            <Suspense fallback={fallback}>
              <Deployments name={name} uploading={uploading} />
            </Suspense>
          </TabsContent>
          <TabsContent value="domains">
            <Suspense fallback={fallback}>
              <Domains name={name} />
            </Suspense>
          </TabsContent>
          <TabsContent value="routes">
            <Suspense fallback={fallback}>
              <PageRoutes app={name} redirectFrom={redirectFrom} />
            </Suspense>
          </TabsContent>
          <TabsContent value="settings">
            <Settings name={name} onDelete={() => setConfirming(true)} />
          </TabsContent>
        </Tabs>
      </PageDropTarget>
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
    </PageTemplate>
  )
}

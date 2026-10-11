import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '../shell/PageHeader'
import { useDomainPolicies } from '../../queries/domainPolicies'
import { AccessTab } from './AccessTab'
import { CacheTab } from './CacheTab'
import { DangerZoneTab } from './DangerZoneTab'
import { ForwardingTab } from './ForwardingTab'
import { HeadersTab } from './HeadersTab'
import { OverviewTab } from './OverviewTab'
import { PortsTab } from './PortsTab'
import { RedirectsTab } from './RedirectsTab'
import { DOMAIN_TABS, type DomainTab } from './domainTabs'

export function DomainPolicyPage({
  app,
  domain,
  tab,
  onTab,
}: {
  app: string
  domain: string
  tab: DomainTab
  onTab: (tab: DomainTab) => void
}) {
  const { t } = useTranslation('domainPolicies')
  const { data, error, isLoading, refetch } = useDomainPolicies(app, domain)

  const header = (
    <PageHeader
      breadcrumb={
        <Link to="/domains" className="hover:underline">
          {t('page.breadcrumb')}
        </Link>
      }
      title={<span className="font-mono">{domain}</span>}
      description={t('page.description', { app })}
    />
  )

  if (!app) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          icon={<WarningCircleIcon className="size-5" />}
          title={t('page.missingAppTitle')}
          description={t('page.missingApp')}
          action={
            <Link
              to="/domains"
              className="text-sm text-primary hover:underline"
            >
              {t('page.backToDomains')}
            </Link>
          }
        />
      </div>
    )
  }
  if (isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-8 w-full max-w-xl" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }
  if (error || !data) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          icon={<WarningCircleIcon className="size-5" />}
          title={t('page.loadFailed')}
          description={error?.message ?? ''}
          action={
            <Button
              type="button"
              variant="outline"
              onClick={() => void refetch()}
            >
              {t('page.retry')}
            </Button>
          }
        />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {header}
      <Tabs value={tab} onValueChange={(v) => onTab(v as DomainTab)}>
        <TabsList variant="line" className="flex-wrap">
          {DOMAIN_TABS.map((k) => (
            <TabsTrigger key={k} value={k}>
              {t(`tabs.${k}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview" className="pt-2">
          <OverviewTab app={app} domain={domain} policies={data} />
        </TabsContent>
        <TabsContent value="redirects" className="pt-2">
          <RedirectsTab app={app} domain={domain} />
        </TabsContent>
        <TabsContent value="headers" className="pt-2">
          <HeadersTab app={app} domain={domain} policies={data} />
        </TabsContent>
        <TabsContent value="forwarding" className="pt-2">
          <ForwardingTab app={app} domain={domain} policies={data} />
        </TabsContent>
        <TabsContent value="access" className="pt-2">
          <AccessTab app={app} domain={domain} policies={data} />
        </TabsContent>
        <TabsContent value="cache" className="pt-2">
          <CacheTab app={app} domain={domain} policies={data} />
        </TabsContent>
        <TabsContent value="ports" className="pt-2">
          <PortsTab app={app} domain={domain} />
        </TabsContent>
        <TabsContent value="danger" className="pt-2">
          <DangerZoneTab app={app} domain={domain} policies={data} />
        </TabsContent>
      </Tabs>
    </div>
  )
}

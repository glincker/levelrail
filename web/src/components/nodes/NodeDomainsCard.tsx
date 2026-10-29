import { useTranslation } from 'react-i18next'
import { InfoGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useNodeDomains } from '../../queries/nodeOnboarding'
import type { DomainGroup, DomainRisk } from '../../types/nodeOnboarding'

const GIB = 1024 ** 3
const CORE_MILLI = 1000

function Capacity({ group }: { group: DomainGroup }) {
  const { t } = useTranslation('nodes')
  return (
    <li className="space-y-0.5 py-2">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <p className="text-sm font-medium text-foreground">
          {group.name}
          {group.providers?.length ? (
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              {group.providers.join(' / ')}
            </span>
          ) : null}
        </p>
        <p className="text-xs text-muted-foreground tabular-nums">
          {t('domains.nodes', {
            online: group.online_nodes,
            total: group.nodes,
          })}
          {', '}
          {t('domains.workloads', { count: group.workloads })}
        </p>
      </div>
      <p className="text-xs text-muted-foreground tabular-nums">
        {t('domains.capacity', {
          cores: (group.cpu_milli / CORE_MILLI).toFixed(1),
          memory: (group.memory_bytes / GIB).toFixed(1),
          disk: (group.disk_free_bytes / GIB).toFixed(0),
        })}
        {group.unverified_nodes > 0
          ? ` ${t('domains.unverified', { count: group.unverified_nodes })}`
          : ''}
      </p>
    </li>
  )
}

function RiskRow({ risk }: { risk: DomainRisk }) {
  const Icon = risk.level === 'warn' ? TriangleAlertGlassIcon : InfoGlassIcon
  return (
    <li className="flex items-start gap-2 text-xs">
      <Icon
        className={`mt-0.5 size-3.5 shrink-0 ${risk.level === 'warn' ? 'text-tone-warning' : 'text-muted-foreground'}`}
      />
      <span className="text-foreground">{risk.message}</span>
    </li>
  )
}

// Capacity per provider, region and zone, plus apps whose replicas all sit in
// one failure domain. Reads only what nodes reported; unreported capacity is
// flagged as unverified rather than counted as zero.
export default function NodeDomainsCard() {
  const { t } = useTranslation('nodes')
  const domains = useNodeDomains()

  if (domains.isPending) {
    return <Skeleton className="h-28 w-full" />
  }
  if (domains.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertDescription className="space-y-2">
          <p>
            {t('domains.loadFailed')} {domains.error.message}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void domains.refetch()}
          >
            {t('domains.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const d = domains.data
  if (d.total_nodes < 2) {
    return null
  }
  const tabs = [
    {
      key: 'providers',
      label: t('domains.tabs.providers'),
      groups: d.providers,
    },
    { key: 'regions', label: t('domains.tabs.regions'), groups: d.regions },
    { key: 'zones', label: t('domains.tabs.zones'), groups: d.zones },
  ].filter((tab) => tab.groups.length > 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('domains.title')}</CardTitle>
        <CardDescription>{t('domains.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {tabs.length === 0 ? (
          <Alert>
            <InfoGlassIcon className="text-tone-info" size="1.2em" />
            <AlertDescription>{t('domains.noLabels')}</AlertDescription>
          </Alert>
        ) : (
          <Tabs defaultValue={tabs[0]?.key}>
            <TabsList>
              {tabs.map((tab) => (
                <TabsTrigger key={tab.key} value={tab.key}>
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {tabs.map((tab) => (
              <TabsContent key={tab.key} value={tab.key}>
                <ul className="divide-y divide-border">
                  {tab.groups.map((g) => (
                    <Capacity key={g.name} group={g} />
                  ))}
                </ul>
              </TabsContent>
            ))}
          </Tabs>
        )}
        {d.unlabeled_nodes > 0 && tabs.length > 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('domains.unlabeled', {
              count: d.unlabeled_nodes,
              total: d.total_nodes,
            })}
          </p>
        ) : null}
        {d.risks.length > 0 ? (
          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">
              {t('domains.risksTitle')}
            </p>
            <ul className="space-y-1.5">
              {d.risks.map((r) => (
                <RiskRow key={`${r.name}:${r.kind}`} risk={r} />
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { HelpLink } from '@/components/HelpLink'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { PageBanner } from '../PageBanner'
import { LazyFirewallDiagram } from '../banners/LazyPass3Diagrams'
import { cn } from '@/lib/utils'
import { domainsQueryOptions } from '../../queries/domains'
import type { FwRange } from '../../queries/firewallEdge'
import { RuleSelect } from '../rules/RuleSelect'

export type FirewallTab = 'overview' | 'traffic' | 'rules' | 'audit'

const RANGES: FwRange[] = ['1h', '6h', '24h']

const TAB_CLASS =
  'relative -mb-px border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none aria-[current=page]:border-foreground aria-[current=page]:font-medium aria-[current=page]:text-foreground'

/** FirewallPage is the shared shell of the four firewall views: hero, tab links, host scope and time range. */
export function FirewallPage({
  tab,
  scope,
  range,
  onScope,
  onRange,
  actions,
  children,
}: {
  tab: FirewallTab
  scope: string
  range: FwRange
  onScope: (scope: string) => void
  onRange: (range: FwRange) => void
  actions?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation('firewall')
  const domains = useQuery(domainsQueryOptions())
  const hosts = [...new Set((domains.data ?? []).map((d) => d.domain))].sort()
  if (scope && !hosts.includes(scope)) hosts.unshift(scope)
  const rangeLabels: Record<FwRange, string> = {
    '1h': t('page.ranges.1h'),
    '6h': t('page.ranges.6h'),
    '24h': t('page.ranges.24h'),
  }
  const search = { scope: scope || undefined, range }
  const current = (id: FirewallTab) => (tab === id ? 'page' : undefined)

  const tabs = (
    <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2 border-b border-border">
      <nav aria-label={t('page.sections')} className="flex flex-wrap">
        <Link
          to="/firewall"
          search={search}
          aria-current={current('overview')}
          className={TAB_CLASS}
        >
          {t('page.tabs.overview')}
        </Link>
        <Link
          to="/firewall/traffic"
          search={search}
          aria-current={current('traffic')}
          className={TAB_CLASS}
        >
          {t('page.tabs.traffic')}
        </Link>
        <Link
          to="/firewall/rules"
          search={search}
          aria-current={current('rules')}
          className={TAB_CLASS}
        >
          {t('page.tabs.rules')}
        </Link>
        <Link
          to="/firewall/audit"
          search={search}
          aria-current={current('audit')}
          className={TAB_CLASS}
        >
          {t('page.tabs.audit')}
        </Link>
      </nav>
      <div className={cn('flex flex-wrap items-center gap-2 pb-2')}>
        <RuleSelect
          aria-label={t('page.scope')}
          value={scope}
          options={[
            { value: '', label: t('page.allHosts') },
            ...hosts.map((h) => ({ value: h, label: h })),
          ]}
          className="max-w-56"
          onChange={(e) => onScope(e.target.value)}
        />
        {tab === 'overview' ? (
          <RuleSelect
            aria-label={t('page.range')}
            value={range}
            options={RANGES.map((r) => ({ value: r, label: rangeLabels[r] }))}
            onChange={(e) => onRange(e.target.value as FwRange)}
          />
        ) : null}
      </div>
    </div>
  )

  return (
    <PageTemplate
      title={t('page.title')}
      width="wide"
      banner={
        <PageBanner
          icon={
            <ShieldCheckGlassIcon className="text-tone-success" size="1.4em" />
          }
          titleKey="firewall:page.title"
          descriptionKey="firewall:page.description"
          action={
            <>
              <HelpLink path="/firewall" label={t('page.help')} />
              {actions}
            </>
          }
          illustration={<LazyFirewallDiagram size="compact" />}
          size="compact"
        />
      }
    >
      {tabs}
      {children}
    </PageTemplate>
  )
}

import { useTranslation } from 'react-i18next'
import { InfoGlassIcon } from '@thesvg/ui-react'
import { useFirewallTraffic, type FwTopTable } from '../../queries/firewallEdge'
import { LoadError } from './LoadError'
import { TopList, TopListSkeleton } from './TopList'
import type { FirewallActions } from './useFirewallActions'

const TABLES: { table: FwTopTable; mono: boolean }[] = [
  { table: 'ip', mono: true },
  { table: 'denied_ip', mono: true },
  { table: 'user_agent', mono: false },
  { table: 'path', mono: true },
  { table: 'host', mono: true },
]

// Tiles fill every row: three across on the first row, the rest share the last, and an odd last tile spans two columns on medium screens.
function spanFor(i: number, n: number): string {
  const wide = i >= 3 ? 'xl:col-span-3' : 'xl:col-span-2'
  return i === n - 1 && n % 2 === 1 ? `md:col-span-2 ${wide}` : wide
}

/** TrafficView shows who and what is hitting the instance: approximate top tables for the last hour. */
export function TrafficView({
  scope,
  actions,
}: {
  scope: string
  actions: FirewallActions
}) {
  const { t } = useTranslation('firewall')
  const { data, error, refetch } = useFirewallTraffic(scope)

  const titles: Record<FwTopTable, string> = {
    ip: t('traffic.ips'),
    denied_ip: t('traffic.deniedIps'),
    user_agent: t('traffic.userAgents'),
    path: t('traffic.paths'),
    host: t('traffic.hosts'),
  }
  const empties: Record<FwTopTable, string> = {
    ip: t('traffic.ipsEmpty'),
    denied_ip: t('traffic.deniedEmpty'),
    user_agent: t('traffic.userAgentsEmpty'),
    path: t('traffic.pathsEmpty'),
    host: t('traffic.hostsEmpty'),
  }

  if (error) return <LoadError error={error} onRetry={() => void refetch()} />

  return (
    <div className="space-y-(--glinr-page-gap)">
      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <InfoGlassIcon
          aria-hidden="true"
          className="mt-0.5 size-5 shrink-0 text-tone-info"
        />
        {t('traffic.note', { minutes: data?.window_minutes ?? 60 })}
      </p>
      <div className="grid gap-(--glinr-page-gap) md:grid-cols-2 xl:grid-cols-6">
        {TABLES.map(({ table, mono }, i) =>
          data ? (
            <TopList
              key={table}
              className={spanFor(i, TABLES.length)}
              title={titles[table]}
              rows={data.tables[table]}
              mono={mono}
              empty={empties[table]}
              menuFor={(row) => actions.menuFor(table, row.key)}
              sparkLabel={(row) => t('topList.spark', { key: row.key })}
            />
          ) : (
            <TopListSkeleton
              key={table}
              className={spanFor(i, TABLES.length)}
            />
          ),
        )}
      </div>
      {actions.dialog}
    </div>
  )
}

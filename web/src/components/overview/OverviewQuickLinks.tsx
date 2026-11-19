import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusShape, type StatusState } from '@/components/kit'
import { useAppHealthScore } from '../../queries/healthScore'
import {
  siteChecksQueryOptions,
  usePagesSiteDetail,
} from '../../queries/pagesSites'
import { formatSize } from '../../lib/format'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { ResourceReading } from './useResourceReading'
import { DEPLOY_STATE } from './staticStripModel'

interface Item {
  key: string
  label: string
  text: string
  state?: StatusState
  to:
    | '/apps/$name/health'
    | '/apps/$name/resources'
    | '/apps/$name/domains'
    | '/apps/$name/deploys'
}

function Row({ item, name }: Readonly<{ item: Item; name: string }>) {
  return (
    <li className="min-w-0">
      <Link
        to={item.to}
        params={{ name }}
        className="flex min-h-9 items-center gap-2 rounded-md border border-border bg-card px-3 py-1.5 text-sm outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring/60 motion-reduce:transition-none"
      >
        {item.state ? <StatusShape state={item.state} /> : null}
        <span className="sr-only">{item.label}: </span>
        <span className="truncate tabular-nums">{item.text}</span>
      </Link>
    </li>
  )
}

/** One quiet row of live-state links to the tabs people otherwise hunt for. */
export function OverviewQuickLinks({
  app,
  reading,
  latest,
  isStatic,
}: Readonly<{
  app: AppDetail
  reading: ResourceReading
  latest?: DeployAttempt
  isStatic: boolean
}>) {
  const { t } = useTranslation('appPages')
  const score = useAppHealthScore(app.name, !isStatic)
  const checks = useQuery({
    ...siteChecksQueryOptions(app.name),
    enabled: isStatic,
  })
  const site = usePagesSiteDetail(app.name, isStatic)

  let health: Pick<Item, 'text' | 'state'> = { text: t('quick.health') }
  if (isStatic && checks.data) {
    const fail = checks.data.checks.filter((c) => c.status === 'fail').length
    health =
      fail > 0
        ? { text: t('quick.failing', { count: fail }), state: 'failed' }
        : {
            text: t('quick.healthScore', { score: checks.data.score }),
            state: 'healthy',
          }
  } else if (!isStatic && score.data) {
    const cats = score.data.categories
    const fail = cats.filter((c) => c.status === 'fail').length
    const warn = cats.filter((c) => c.status === 'warn').length
    if (fail > 0)
      health = { text: t('quick.failing', { count: fail }), state: 'failed' }
    else if (warn > 0)
      health = { text: t('quick.warning', { count: warn }), state: 'degraded' }
    else health = { text: t('quick.healthPass'), state: 'healthy' }
  }

  let resources: Pick<Item, 'text'> = { text: t('quick.resources') }
  if (isStatic && site.data) {
    resources = {
      text: t('quick.disk', { size: formatSize(site.data.size_bytes) }),
    }
  } else if (!isStatic && reading.cpuNow !== null) {
    resources = { text: t('quick.cpu', { value: reading.cpuNow.toFixed(0) }) }
  }

  const domainCount = app.domains?.length ?? 0
  const items: Item[] = [
    {
      key: 'health',
      label: t('quick.health'),
      to: '/apps/$name/health',
      ...health,
    },
    {
      key: 'resources',
      label: t('quick.resources'),
      to: '/apps/$name/resources',
      ...resources,
    },
    {
      key: 'domains',
      label: t('quick.domains'),
      to: '/apps/$name/domains',
      text:
        domainCount > 0
          ? t('quick.domainCount', { count: domainCount })
          : t('quick.noDomains'),
    },
    {
      key: 'deploys',
      label: t('quick.deploys'),
      to: '/apps/$name/deploys',
      state: latest ? DEPLOY_STATE[latest.status] : undefined,
      text: latest
        ? t('quick.deployLatest', {
            status: t(`quick.deployStatus.${latest.status}`),
          })
        : t('quick.noDeploys'),
    },
  ]
  return (
    <nav aria-label={t('quick.label')}>
      <ul className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {items.map((i) => (
          <Row key={i.key} item={i} name={app.name} />
        ))}
      </ul>
    </nav>
  )
}

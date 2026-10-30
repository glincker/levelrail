import { Link } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GlobeGlassIcon,
  PlusGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { sortByCertAttention } from '../lib/certStatus'
import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'
import {
  DomainCertCell,
  DomainDnsCell,
  DomainExpiryCell,
  DomainNameCell,
  DomainRowActions,
  DomainTargetCell,
  DomainVisibilityCell,
} from './DomainRow'

export function DomainsTableSkeleton() {
  return <TableSkeleton columnCount={6} rowCount={6} />
}

export function DomainsTable({
  domains,
  certByDomain,
  appCount,
  onAdd,
}: {
  domains: Domain[]
  certByDomain: ReadonlyMap<string, CertificateStatus>
  appCount: number
  onAdd: () => void
}) {
  const { t } = useTranslation('domains')
  const [query, setQuery] = useState('')

  // Problem certificates first so they surface without scrolling.
  const sorted = useMemo(
    () => sortByCertAttention(domains, (d) => certByDomain.get(d.domain)),
    [domains, certByDomain],
  )
  const needle = query.trim().toLowerCase()
  const visible = useMemo(
    () =>
      needle === ''
        ? sorted
        : sorted.filter(
            (d) =>
              d.domain.toLowerCase().includes(needle) ||
              d.service_name.toLowerCase().includes(needle) ||
              (d.target ?? '').toLowerCase().includes(needle),
          ),
    [sorted, needle],
  )

  const columns = useMemo<ResponsiveColumn<Domain>[]>(
    () => [
      {
        id: 'domain',
        primary: true,
        header: t('page.col.domain'),
        cell: (d) => (
          <DomainNameCell domain={d} cert={certByDomain.get(d.domain)} />
        ),
      },
      {
        id: 'app',
        header: t('page.col.app'),
        width: 'w-44',
        cell: (d) => <DomainTargetCell domain={d} />,
      },
      {
        id: 'dns',
        header: t('page.col.dns'),
        priority: 2,
        width: 'w-28',
        cell: (d) => <DomainDnsCell domain={d} />,
      },
      {
        id: 'certificate',
        header: t('page.col.certificate'),
        priority: 2,
        width: 'w-36',
        cell: (d) => (
          <DomainCertCell domain={d} cert={certByDomain.get(d.domain)} />
        ),
      },
      {
        id: 'expires',
        header: t('page.col.expires'),
        priority: 3,
        width: 'w-40',
        cell: (d) => (
          <DomainExpiryCell domain={d} cert={certByDomain.get(d.domain)} />
        ),
      },
      {
        id: 'visibility',
        header: t('page.col.visibility'),
        priority: 3,
        width: 'w-20',
        cell: (d) => <DomainVisibilityCell domain={d} />,
      },
    ],
    [t, certByDomain],
  )

  if (domains.length === 0) {
    return appCount === 0 ? (
      <EmptyState
        icon={<GlobeGlassIcon className="size-6" />}
        title={t('page.empty.noAppsTitle')}
        description={t('page.empty.noAppsBody')}
        action={
          <div className="flex flex-wrap justify-center gap-2">
            <Button size="sm" render={<Link to="/apps" />} nativeButton={false}>
              {t('page.empty.noAppsAction')}
            </Button>
            <Button size="sm" variant="outline" onClick={onAdd}>
              <PlusGlassIcon size="1.4em" />
              {t('standalone.modeProxy')}
            </Button>
          </div>
        }
      />
    ) : (
      <EmptyState
        icon={<GlobeGlassIcon className="size-6" />}
        title={t('page.empty.title')}
        description={t('page.empty.body')}
        action={
          <Button size="sm" onClick={onAdd}>
            <PlusGlassIcon size="1.4em" />
            {t('page.empty.action')}
          </Button>
        }
      />
    )
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="relative w-full max-w-xs">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            className="pl-8"
            value={query}
            placeholder={t('page.search')}
            aria-label={t('page.search')}
            onChange={(e) => {
              setQuery(e.target.value)
            }}
          />
        </div>
        <span className="text-xs text-muted-foreground">
          {t('page.count', { count: visible.length })}
        </span>
      </div>

      {visible.length === 0 ? (
        <EmptyState
          icon={<SearchGlassIcon className="size-6" />}
          title={t('page.noMatch', { query: query.trim() })}
          description={t('page.noMatchBody')}
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setQuery('')
              }}
            >
              {t('page.clearSearch')}
            </Button>
          }
        />
      ) : (
        <ResponsiveTable
          rows={visible}
          columns={columns}
          getRowId={(d) => `${d.service_name}/${d.domain}`}
          ariaLabel={t('page.tableLabel')}
          renderActions={(d) => <DomainRowActions domain={d} />}
          estimateRowHeight={60}
        />
      )}
    </div>
  )
}

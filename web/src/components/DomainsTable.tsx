import { Link, useNavigate } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeftRightGlassIcon,
  GlobeGlassIcon,
  PauseGlassIcon,
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
import { canParkOrMove } from '../lib/domainLifecycle'
import { domainScope, isStandaloneDomain } from '../lib/standaloneDomain'
import {
  resolveSelection,
  selectAllState,
  setKeys,
  toggleKey,
  type BatchAction,
} from '../lib/domainsBatch'
import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'
import { useDomainsWatchRefresh } from '../queries/domainsWatch'
import { DomainsBatchDialog } from './domains/DomainsBatchDialog'
import {
  DomainsBulkBar,
  RowSelectBox,
  SelectAllBox,
} from './domains/DomainsBulkBar'
import {
  DomainCertCell,
  DomainDnsCell,
  DomainEnvironmentCell,
  DomainExpiryCell,
  DomainNameCell,
  DomainRowActions,
  DomainTargetCell,
  DomainVisibilityCell,
} from './DomainRow'

export function DomainsTableSkeleton() {
  return <TableSkeleton columnCount={7} rowCount={6} />
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
  useDomainsWatchRefresh()
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [batchAction, setBatchAction] = useState<BatchAction | null>(null)

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

  const selectable = useMemo(
    () => visible.filter(canParkOrMove).map((d) => d.domain),
    [visible],
  )
  const chosenRows = useMemo(
    () => resolveSelection(selected, domains, (d) => d.domain, canParkOrMove),
    [selected, domains],
  )
  const headerState = selectAllState(selected, selectable)

  const allColumns = useMemo<ResponsiveColumn<Domain>[]>(
    () => [
      {
        id: 'select',
        header: (
          <SelectAllBox
            state={headerState}
            disabled={selectable.length === 0}
            onChange={(on) => {
              setSelected((s) => setKeys(s, selectable, on))
            }}
          />
        ),
        cardLabel: t('bulk.select'),
        width: 'w-10',
        cell: (d) => (
          <RowSelectBox
            domain={d.domain}
            checked={selected.has(d.domain)}
            blocked={
              canParkOrMove(d)
                ? undefined
                : d.automatic
                  ? t('bulk.blockedAutomatic')
                  : t('bulk.blockedStandalone')
            }
            onChange={() => {
              setSelected((s) => toggleKey(s, d.domain))
            }}
          />
        ),
      },
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
        id: 'environment',
        header: t('page.col.environment'),
        priority: 3,
        width: 'w-28',
        cell: (d) => <DomainEnvironmentCell domain={d} />,
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
    [t, certByDomain, selected, selectable, headerState],
  )

  // An all-dash column is noise, so it only shows when some row has a value.
  const columns = useMemo(
    () =>
      domains.some((d) => d.environment)
        ? allColumns
        : allColumns.filter((c) => c.id !== 'environment'),
    [allColumns, domains],
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
            className="pointer-events-none absolute top-1/2 left-3 size-4.5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            className="pl-10"
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

      <DomainsBulkBar
        count={chosenRows.length}
        actions={[
          {
            id: 'park',
            label: t('bulk.park'),
            icon: <PauseGlassIcon size="1.4em" aria-hidden="true" />,
            onClick: () => {
              setBatchAction('park')
            },
          },
          {
            id: 'move',
            label: t('bulk.move'),
            icon: <ArrowLeftRightGlassIcon size="1.4em" aria-hidden="true" />,
            onClick: () => {
              setBatchAction('move')
            },
          },
        ]}
        onClear={() => {
          setSelected(new Set())
        }}
      />

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
          onRowClick={(d) => {
            void (isStandaloneDomain(d)
              ? navigate({
                  to: '/domains/$domain',
                  params: { domain: d.domain },
                  search: { app: domainScope(d) },
                })
              : navigate({
                  to: '/apps/$name/domains/$domain',
                  params: { name: domainScope(d), domain: d.domain },
                }))
          }}
          estimateRowHeight={60}
        />
      )}
      {batchAction ? (
        <DomainsBatchDialog
          action={batchAction}
          subjects={chosenRows}
          open
          onOpenChange={(o) => {
            if (!o) setBatchAction(null)
          }}
          onDone={(ok) => {
            setSelected((s) => setKeys(s, ok, false))
          }}
        />
      ) : null}
    </div>
  )
}

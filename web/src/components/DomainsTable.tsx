import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GlobeIcon,
  MagnifyingGlassIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { sortByCertAttention } from '../lib/certStatus'
import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'
import { DOMAIN_LIST_GRID, DomainRow, RowSkeleton } from './DomainRow'

// Rows are virtualized: a platform can route far more than 50 domains.
const ROW_HEIGHT = 60

export function DomainsTableHeader() {
  const { t } = useTranslation('domains')
  return (
    <div
      className={`${DOMAIN_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span>{t('page.col.domain')}</span>
      <span>{t('page.col.app')}</span>
      <span>{t('page.col.dns')}</span>
      <span>{t('page.col.certificate')}</span>
      <span>{t('page.col.expires')}</span>
      <span>{t('page.col.visibility')}</span>
      <span aria-hidden="true" />
    </div>
  )
}

export function DomainsTableSkeleton() {
  return (
    <div className="overflow-x-auto rounded-lg border border-border bg-card">
      <DomainsTableHeader />
      {Array.from({ length: 6 }, (_, i) => (
        <RowSkeleton key={i} />
      ))}
    </div>
  )
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
  const parentRef = useRef<HTMLDivElement>(null)

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
              d.service_name.toLowerCase().includes(needle),
          ),
    [sorted, needle],
  )

  const virtualizer = useVirtualizer({
    count: visible.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  if (domains.length === 0) {
    return appCount === 0 ? (
      <EmptyState
        icon={<GlobeIcon className="size-5" />}
        title={t('page.empty.noAppsTitle')}
        description={t('page.empty.noAppsBody')}
        action={
          <Button size="sm" render={<Link to="/apps" />} nativeButton={false}>
            {t('page.empty.noAppsAction')}
          </Button>
        }
      />
    ) : (
      <EmptyState
        icon={<GlobeIcon className="size-5" />}
        title={t('page.empty.title')}
        description={t('page.empty.body')}
        action={
          <Button size="sm" onClick={onAdd}>
            <PlusIcon />
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
          <MagnifyingGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
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
          icon={<MagnifyingGlassIcon className="size-5" />}
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
        <div
          ref={parentRef}
          className="max-h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <DomainsTableHeader />
          <div
            className="relative"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((row) => {
              const domain = visible[row.index]
              if (!domain) return null
              return (
                <div
                  key={`${domain.service_name}/${domain.domain}`}
                  className="absolute top-0 left-0 w-full"
                  style={{
                    height: row.size,
                    transform: `translateY(${row.start}px)`,
                  }}
                >
                  <DomainRow
                    domain={domain}
                    cert={certByDomain.get(domain.domain)}
                  />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

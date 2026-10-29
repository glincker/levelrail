import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, Share2GlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import {
  useTrafficExportSinks,
  useTrafficExportStatus,
} from '../../queries/trafficExport'
import type { TrafficExportSink } from '../../types/trafficExport'
import { TrafficExportRow } from './TrafficExportRow'
import { TrafficExportSheet } from './TrafficExportSheet'

type SheetState = { open: false } | { open: true; sink?: TrafficExportSink }

/** TrafficExportCard lists the export destinations with live status chips and opens the add and edit panel. */
export function TrafficExportCard() {
  const { t } = useTranslation('trafficExport')
  const sinks = useTrafficExportSinks()
  const status = useTrafficExportStatus()
  const [sheet, setSheet] = useState<SheetState>({ open: false })
  const notConfigured =
    status.error instanceof ApiError && status.error.status === 501
  const byId = new Map((status.data?.sinks ?? []).map((s) => [s.id, s]))

  return (
    <section className="rounded-xl bg-linear-to-b from-card to-muted/50 text-card-foreground shadow-card ring-1 ring-foreground/10">
      <header className="flex flex-wrap items-start justify-between gap-3 px-4 pt-4">
        <div className="min-w-0">
          <h2 className="text-sm font-medium">{t('card.title')}</h2>
          <p className="text-sm text-muted-foreground">
            {t('card.description')}
          </p>
        </div>
        <Button
          size="sm"
          disabled={notConfigured}
          onClick={() => {
            setSheet({ open: true })
          }}
        >
          <PlusGlassIcon className="size-5" aria-hidden="true" />
          {t('add')}
        </Button>
      </header>
      <div className="px-4 pb-2">
        {sinks.isPending ? (
          <div className="space-y-2 py-3" aria-hidden="true">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : null}
        {notConfigured ? (
          <Banner
            variant="warning"
            title={t('notConfigured')}
            className="my-3"
          />
        ) : null}
        {sinks.error ? (
          <Banner
            variant="critical"
            title={t('loadError', { message: sinks.error.message })}
            className="my-3"
          />
        ) : null}
        {sinks.data && sinks.data.sinks.length === 0 ? (
          <EmptyState
            icon={<Share2GlassIcon className="size-6" />}
            title={t('empty.title')}
            description={t('empty.description')}
            className="border-0 bg-transparent py-6"
          />
        ) : null}
        {sinks.data && sinks.data.sinks.length > 0 ? (
          <ul className="divide-y divide-border">
            {sinks.data.sinks.map((s) => (
              <TrafficExportRow
                key={s.id}
                sink={s}
                status={byId.get(s.id)}
                onEdit={(sink) => {
                  setSheet({ open: true, sink })
                }}
              />
            ))}
          </ul>
        ) : null}
      </div>
      <p className="border-t border-border px-4 py-3 text-xs text-muted-foreground">
        {t('privacy')}
      </p>
      {sheet.open ? (
        <TrafficExportSheet
          key={sheet.sink?.id ?? 'new'}
          open
          sink={sheet.sink}
          onOpenChange={(open) => {
            if (!open) setSheet({ open: false })
          }}
        />
      ) : null}
    </section>
  )
}

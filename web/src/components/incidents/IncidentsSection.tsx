import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import { useIncidents } from '../../queries/incidents'
import { useMonitors } from '../../queries/monitors'
import type { Incident, IncidentInternalState } from '../../types/incident'
import { IncidentDetail } from './IncidentDetail'

const STATE_VARIANT: Record<
  IncidentInternalState,
  'destructive' | 'warning' | 'success'
> = {
  triggered: 'destructive',
  acknowledged: 'warning',
  resolved: 'success',
}

export function IncidentsList({
  incidents,
  monitorName,
  onOpen,
}: {
  incidents: Incident[]
  monitorName: (inc: Incident) => string | undefined
  onOpen: (id: string) => void
}) {
  const { t } = useTranslation('monitors')
  if (incidents.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('incidents.empty')}</p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-lg border border-border bg-card">
      {incidents.map((inc) => (
        <li
          key={inc.id}
          className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm"
        >
          <Badge variant={STATE_VARIANT[inc.internal_state]}>
            {t(`incidents.state.${inc.internal_state}`)}
          </Badge>
          <button
            type="button"
            onClick={() => {
              onOpen(inc.id)
            }}
            className="min-w-40 flex-1 rounded text-left font-medium break-words hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            {inc.title}
          </button>
          <span className="w-32 truncate text-xs text-muted-foreground">
            {monitorName(inc) ?? ''}
          </span>
          <span
            className="w-28 text-xs text-muted-foreground"
            title={formatAbsoluteTime(inc.starts_at)}
          >
            {formatRelativeTime(inc.starts_at)}
          </span>
          {inc.published ? (
            <Badge variant="info">{t('incidents.published')}</Badge>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

// Hidden entirely when the API answers 403, 404 or 501 (incidents off or no ability).
export function IncidentsSection() {
  const { t } = useTranslation('monitors')
  const query = useIncidents()
  const monitors = useMonitors()
  const [openId, setOpenId] = useState<string | null>(null)

  if (!query.isError && !query.data) return null

  const monitorName = (inc: Incident): string | undefined => {
    if (inc.source !== 'monitor' || !inc.source_ref) return undefined
    return monitors.data?.find((m) => m.id === inc.source_ref)?.name
  }
  const open = query.data?.find((i) => i.id === openId)

  return (
    <section aria-labelledby="incidents-title" className="space-y-2">
      <h2 id="incidents-title" className="text-sm font-medium">
        {t('incidents.title')}
      </h2>
      {query.isError ? (
        <div
          role="alert"
          className="flex items-center gap-2 text-sm text-destructive"
        >
          {t('incidents.loadError', { message: query.error.message })}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            {t('incidents.refresh')}
          </Button>
        </div>
      ) : (
        <IncidentsList
          incidents={query.data ?? []}
          monitorName={monitorName}
          onOpen={setOpenId}
        />
      )}
      <Sheet
        open={openId !== null}
        onOpenChange={(o) => {
          if (!o) setOpenId(null)
        }}
      >
        <SheetContent className="overflow-y-auto p-4 sm:max-w-lg">
          <SheetHeader className="p-0">
            <SheetTitle>{open?.title ?? t('incidents.title')}</SheetTitle>
            <SheetDescription>
              {open ? (monitorName(open) ?? t('incidents.detailHint')) : ''}
            </SheetDescription>
          </SheetHeader>
          {openId ? (
            <IncidentDetail
              id={openId}
              monitorName={open ? monitorName(open) : undefined}
            />
          ) : null}
        </SheetContent>
      </Sheet>
    </section>
  )
}

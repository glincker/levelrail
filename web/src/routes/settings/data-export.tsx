import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, ShareGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { KINDS } from '../../lib/dataExportForm'
import { useDataExportDestinations } from '../../queries/dataExport'
import type { DataExportKind } from '../../types/dataExport'
import { DataExportDestinationCard } from '../../components/dataExport/DataExportDestinationCard'
import { CreateDataExportDestinationDialog } from '../../components/dataExport/CreateDataExportDestinationDialog'
import { DestinationGallery } from '../../components/dataExport/DestinationGallery'
import { ScrapeEndpointCard } from '../../components/dataExport/ScrapeEndpointCard'

// Short names used by deep links from other pages, plus every real kind.
const ADD_ALIASES: Record<string, DataExportKind> = {
  datadog: 'logs_datadog',
  loki: 'logs_loki',
  otlp: 'metrics_otlp',
  opentelemetry: 'metrics_otlp',
}

export const Route = createFileRoute('/settings/data-export')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { add?: DataExportKind } => {
    const raw = typeof search.add === 'string' ? search.add : ''
    const add = ADD_ALIASES[raw] ?? KINDS.find((k) => k === raw)
    return add ? { add } : {}
  },
  component: DataExportPage,
})

function DataExportPage() {
  const { t } = useTranslation('dataExport')
  const { add } = Route.useSearch()
  const { data, isPending, error } = useDataExportDestinations()
  const notConfigured = error instanceof ApiError && error.status === 501
  const [open, setOpen] = useState(add !== undefined)
  const [kind, setKind] = useState<DataExportKind | undefined>(add)

  return (
    <PageTemplate
      icon={<ShareGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      actions={
        notConfigured ? null : (
          <Button
            onClick={() => {
              setOpen(true)
            }}
          >
            <PlusGlassIcon className="size-5" aria-hidden="true" />
            {t('add')}
          </Button>
        )
      }
    >
      {isPending ? (
        <div className="space-y-2" aria-hidden="true">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : null}

      {notConfigured ? (
        <Banner variant="critical" title={t('notConfigured')} />
      ) : null}

      {error && !notConfigured ? (
        <Banner
          variant="critical"
          title={t('loadError', { message: error.message })}
        />
      ) : null}

      {data && data.length === 0 ? (
        <section className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t('empty.description')}
          </p>
          <DestinationGallery
            onPick={(k) => {
              setKind(k)
              setOpen(true)
            }}
          />
        </section>
      ) : null}

      {data && data.length > 0 ? (
        <ul className="divide-y rounded-lg border">
          {data.map((d) => (
            <DataExportDestinationCard key={d.id} destination={d} />
          ))}
        </ul>
      ) : null}

      <ScrapeEndpointCard />

      {notConfigured || !open ? null : (
        <CreateDataExportDestinationDialog
          open={open}
          onOpenChange={(next) => {
            setOpen(next)
            if (!next) setKind(undefined)
          }}
          initialKind={kind}
        />
      )}
    </PageTemplate>
  )
}

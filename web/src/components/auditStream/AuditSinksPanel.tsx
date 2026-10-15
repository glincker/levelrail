import { useTranslation } from 'react-i18next'
import { BroadcastIcon } from '@phosphor-icons/react/dist/ssr'
import { Banner } from '@/components/ui/banner'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { useAuditSinks } from '../../queries/auditStream'
import { AuditSinkCard } from './AuditSinkCard'
import { AuditSinkFormDialog } from './AuditSinkFormDialog'

export function AuditSinksPanel() {
  const { t } = useTranslation('auditStream')
  const { data, isPending, error } = useAuditSinks()
  const notConfigured = error instanceof ApiError && error.status === 501

  return (
    <section className="space-y-4" aria-labelledby="audit-sinks-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <h2
            id="audit-sinks-title"
            className="text-base font-medium text-foreground"
          >
            {t('sinks.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('sinks.description')}
          </p>
        </div>
        {notConfigured ? null : <AuditSinkFormDialog />}
      </div>

      {isPending ? <Skeleton className="h-40 w-full" /> : null}

      {notConfigured ? (
        <Banner variant="critical" title={t('sinks.notConfigured')} />
      ) : null}

      {error && !notConfigured ? (
        <Banner
          variant="critical"
          title={t('sinks.loadError', { message: error.message })}
        />
      ) : null}

      {data && data.length === 0 ? (
        <EmptyState
          icon={<BroadcastIcon className="size-5" />}
          title={t('sinks.empty.title')}
          description={t('sinks.empty.description')}
          className="border-0 bg-transparent py-6"
        />
      ) : null}

      {data && data.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {data.map((k) => (
            <AuditSinkCard key={k.id} sink={k} />
          ))}
        </div>
      ) : null}
    </section>
  )
}

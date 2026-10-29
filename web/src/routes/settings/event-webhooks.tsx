import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { WebhookGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { useEventWebhooks } from '../../queries/eventWebhooks'
import { EventWebhookCard } from '../../components/eventWebhooks/EventWebhookCard'
import { CreateEventWebhookDialog } from '../../components/eventWebhooks/CreateEventWebhookDialog'

export const Route = createFileRoute('/settings/event-webhooks')({
  component: EventWebhooksPage,
})

function EventWebhooksPage() {
  const { t } = useTranslation('eventWebhooks')
  const { data, isPending, error } = useEventWebhooks()
  const notConfigured = error instanceof ApiError && error.status === 501

  return (
    <PageTemplate
      icon={<WebhookGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      actions={notConfigured ? null : <CreateEventWebhookDialog />}
    >
      {isPending ? (
        <div className="space-y-4" aria-hidden="true">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-40 w-full" />
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
        <EmptyState
          icon={<WebhookGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
          className="border-0 bg-transparent py-6"
        />
      ) : null}

      {data && data.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {data.map((h) => (
            <EventWebhookCard key={h.id} webhook={h} />
          ))}
        </div>
      ) : null}
    </PageTemplate>
  )
}

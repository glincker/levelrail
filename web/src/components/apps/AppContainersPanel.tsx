import { useTranslation } from 'react-i18next'
import { ContainerGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { EmptyState } from '@/components/ui/empty-state'
import { ApiErrorAlert } from '../ApiErrorAlert'
import { ApiError } from '../../lib/apiError'
import { summarizeAppStatus } from '../../lib/appStatus'
import { useAppContainers } from '../../queries/appContainers'
import { useAppStatus } from '../../queries/appStatus'
import { AppContainerCard } from './AppContainerCard'

function EmptyContainers({ name }: { name: string }) {
  const { t } = useTranslation('appContainers')
  const { data } = useAppStatus(name)
  const status = summarizeAppStatus(data?.conditions ?? []).label
  return (
    <EmptyState
      compact
      icon={<ContainerGlassIcon className="size-6" />}
      title={t('empty.title')}
      description={t('empty.description', { status })}
    />
  )
}

export function AppContainersPanel({ name }: { name: string }) {
  const { t } = useTranslation('appContainers')
  const { data, isPending, error, dataUpdatedAt } = useAppContainers(name)

  if (isPending) {
    return (
      <div className="space-y-3" role="status" aria-label={t('loading')}>
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }
  if (error) {
    if (error instanceof ApiError && error.status === 403) {
      return (
        <p role="alert" className="text-sm text-muted-foreground">
          {t('forbidden')}
        </p>
      )
    }
    return <ApiErrorAlert error={error} />
  }
  if (data.length === 0) return <EmptyContainers name={name} />

  return (
    <div className="space-y-3">
      <h2 className="text-sm font-medium text-muted-foreground">
        {t('title')} · {t('count', { count: data.length })}
      </h2>
      {data.map((c) => (
        <AppContainerCard key={c.name} c={c} now={dataUpdatedAt} />
      ))}
    </div>
  )
}

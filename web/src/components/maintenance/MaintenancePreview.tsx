import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { useMaintenancePreview } from '../../queries/domainMaintenance'

const DEBOUNCE_MS = 400

// The page visitors get, rendered by the server and shown in a sandboxed
// frame with no scripts, so operator text can never run in the dashboard.
export function MaintenancePreview({
  title,
  message,
  retryAfterSeconds,
}: {
  title: string
  message: string
  retryAfterSeconds: number
}) {
  const { t } = useTranslation('maintenance')
  const [debounced, setDebounced] = useState({
    title,
    message,
    retryAfterSeconds,
  })
  useEffect(() => {
    const id = setTimeout(() => {
      setDebounced({ title, message, retryAfterSeconds })
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(id)
    }
  }, [title, message, retryAfterSeconds])
  const { data, isError } = useMaintenancePreview(
    debounced.title,
    debounced.message,
    debounced.retryAfterSeconds,
  )

  return (
    <figure className="space-y-1">
      <figcaption className="text-xs text-muted-foreground">
        {data
          ? t('preview.caption', {
              status: data.status,
              retry: data.retry_after,
            })
          : t('preview.title')}
      </figcaption>
      {data ? (
        <iframe
          title={t('preview.frameTitle')}
          sandbox=""
          srcDoc={data.html}
          className="h-44 w-full rounded-md border border-border bg-background"
        />
      ) : isError ? (
        <p className="text-xs text-destructive">{t('preview.failed')}</p>
      ) : (
        <Skeleton className="h-44 w-full" />
      )}
    </figure>
  )
}

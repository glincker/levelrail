import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ApiError } from '@/lib/apiError'

/** LoadError explains a failed load and offers the retry; a 403 says which permission is missing. */
export function LoadError({
  error,
  onRetry,
  compact = false,
}: {
  error: Error
  onRetry: () => void
  compact?: boolean
}) {
  const { t } = useTranslation('firewall')
  const forbidden = error instanceof ApiError && error.status === 403
  return (
    <EmptyState
      compact={compact}
      icon={<CircleAlertGlassIcon className="size-6 text-tone-danger" />}
      title={forbidden ? t('error.forbiddenTitle') : t('error.loadFailed')}
      description={forbidden ? t('error.forbiddenHelp') : error.message}
      action={
        forbidden ? undefined : (
          <Button type="button" size="sm" variant="outline" onClick={onRetry}>
            {t('retry')}
          </Button>
        )
      }
    />
  )
}

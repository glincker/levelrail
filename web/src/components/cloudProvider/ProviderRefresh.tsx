import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { isProviderScopeMissing } from '../../lib/providerCapabilities'
import { RateLimitedError } from '../../queries/providerCapabilities'

export function RefreshButton({
  pending,
  onClick,
}: Readonly<{ pending: boolean; onClick: () => void }>) {
  const { t } = useTranslation('cloudProvider')
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={pending}
      onClick={onClick}
    >
      <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
      {pending ? t('refresh.refreshing') : t('refresh.label')}
    </Button>
  )
}

// A 429 is expected cooldown, not a failure, so it reads as a calm status.
export function RefreshFeedback({ error }: Readonly<{ error: unknown }>) {
  const { t } = useTranslation('cloudProvider')
  if (!error) return null
  if (error instanceof RateLimitedError) {
    return (
      <p role="status" className="text-xs text-tone-warning">
        {error.retryAfterSeconds > 0
          ? t('refresh.rateLimited', { seconds: error.retryAfterSeconds })
          : t('refresh.rateLimitedNoWait')}
      </p>
    )
  }
  if (isProviderScopeMissing(error)) return null
  return (
    <p role="alert" className="text-xs text-tone-danger">
      {t('refresh.error', {
        message: error instanceof Error ? error.message : '',
      })}
    </p>
  )
}

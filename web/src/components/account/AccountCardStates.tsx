import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

export function CardLoading({ rows = 2 }: { rows?: number }) {
  const { t } = useTranslation('settings')
  return (
    <div
      className="space-y-2"
      role="status"
      aria-label={t('accountPage.loading')}
    >
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  )
}

export function CardError({
  onRetry,
  retrying = false,
}: {
  onRetry: () => void
  retrying?: boolean
}) {
  const { t } = useTranslation('settings')
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.7em"
        aria-hidden="true"
      />
      <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
        <span>{t('accountPage.loadError')}</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={retrying}
          onClick={onRetry}
        >
          {t('accountPage.retry')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}

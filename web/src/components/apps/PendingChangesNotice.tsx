import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { useApplyPending, usePendingChanges } from '../../queries/appTimeline'

/** Compact notice for env or secret changes saved but not yet running (for example after a stack apply). */
export function PendingChangesNotice({
  appName,
}: Readonly<{ appName: string }>) {
  const { t } = useTranslation('deploys')
  const { data } = usePendingChanges(appName)
  const apply = useApplyPending(appName)
  if (!data?.pending) return null
  const count = Math.max(
    data.changes.reduce((sum, c) => sum + (c.keys?.length ?? 1), 0),
    1,
  )
  const restart = data.apply_action === 'restart'
  return (
    <Alert role="status" className="flex items-center gap-2 py-1.5">
      <TriangleAlertGlassIcon
        className="size-5 shrink-0 text-tone-warning"
        aria-hidden="true"
      />
      <AlertDescription className="flex flex-1 flex-wrap items-center gap-x-2">
        <span className="font-medium">{t('pendingChanges.title')}</span>
        <span className="text-muted-foreground">
          {t('pendingChanges.detail', { count })}
        </span>
        {apply.isError ? (
          <span className="text-destructive">{t('pendingChanges.failed')}</span>
        ) : null}
      </AlertDescription>
      <Button
        size="sm"
        variant="outline"
        disabled={apply.isPending}
        onClick={() => {
          apply.mutate()
        }}
      >
        <RotateCwGlassIcon className="size-5" aria-hidden="true" />
        {apply.isPending
          ? t('pendingChanges.applying')
          : t(restart ? 'pendingChanges.restart' : 'pendingChanges.redeploy')}
      </Button>
    </Alert>
  )
}

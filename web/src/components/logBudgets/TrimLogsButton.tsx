import { useTranslation } from 'react-i18next'
import { ScissorsGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useTrimLogBudget } from '../../queries/logBudgets'

/** TrimLogsButton is the Fix for an app over its log budget: it trims the oldest lines now. */
export function TrimLogsButton({ app }: { app: string }) {
  const { t } = useTranslation('logBudgets')
  const trim = useTrimLogBudget(app)
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={trim.isPending}
      onClick={() =>
        trim.mutate(undefined, {
          onSuccess: (res) =>
            toast.add({
              title: t('trimDone', { count: res.trimmed_lines }),
              description: res.still_over ? t('trimStillOver') : undefined,
              type: 'success',
            }),
          onError: (err) =>
            toast.add({
              title: t('trimFailed'),
              description: err.message,
              type: 'error',
            }),
        })
      }
    >
      <ScissorsGlassIcon size="1.2em" aria-hidden="true" />
      {t('trimNow')}
    </Button>
  )
}

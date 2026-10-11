import { useTranslation } from 'react-i18next'
import { ClockClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useExtendPreviewEnvironment } from '../queries/previewExtend'

const EXTEND_HOURS = 24

// ExtendPreviewButton keeps one preview alive for another day, the quick
// answer to "this review is not finished and the TTL is close".
export function ExtendPreviewButton({
  appName,
  prNumber,
}: {
  appName: string
  prNumber: number
}) {
  const { t } = useTranslation('previews')
  const extend = useExtendPreviewEnvironment(appName)

  function run() {
    extend.mutate(
      { prNumber, hours: EXTEND_HOURS },
      {
        onSuccess: (preview) => {
          toast.add({
            title: t('extend.success', {
              number: prNumber,
              when: preview.expires_at ?? '',
            }),
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: t('extend.error'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={extend.isPending}
      aria-label={t('extend.label', { number: prNumber })}
      title={t('extend.hours', { hours: EXTEND_HOURS })}
      onClick={run}
    >
      <ClockClockwiseIcon />
      {t('extend.button')}
    </Button>
  )
}

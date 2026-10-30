import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BellOffGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDismissAttentionItem } from '../../queries/deviceAuth'
import { snoozeNow } from '../shell/snoozeStore'

// Open, snooze for a day and dismiss for one failed copy attempt. Both are
// remembered per attempt id, so a new failed attempt shows again.
export function CopyItemControls({
  itemId,
  dismissKey,
  to,
}: {
  itemId: string
  dismissKey: string
  to: string
}) {
  const { t } = useTranslation('attention', { useSuspense: false })
  const dismiss = useDismissAttentionItem()
  return (
    <div className="flex items-center gap-2">
      <Button size="sm" variant="outline" render={<Link to={to} />}>
        {t('copyFailure.openDatabase')}
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('copyFailure.snooze')}
        title={t('copyFailure.snooze')}
        onClick={() => {
          snoozeNow(itemId, '1d')
          toast.add({ title: t('copyFailure.snoozed'), type: 'info' })
        }}
      >
        <BellOffGlassIcon size="1.4em" />
      </Button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('copyFailure.dismiss')}
        title={t('copyFailure.dismiss')}
        disabled={dismiss.isPending}
        onClick={() => {
          dismiss.mutate(dismissKey, {
            onSuccess: () => {
              toast.add({ title: t('copyFailure.dismissed'), type: 'success' })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        }}
      >
        <XGlassIcon size="1.4em" />
      </Button>
    </div>
  )
}

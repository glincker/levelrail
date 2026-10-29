import { useTranslation } from 'react-i18next'
import { BellOffGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast, toastAction } from '@/components/ui/toast'
import { snoozeNow, unsnoozeNow } from '../shell/snoozeStore'

/** SnoozeButton hides one or more attention items for a day with an undo; critical items never get one. */
export function SnoozeButton({ ids }: { ids: string[] }) {
  const { t } = useTranslation('dashboard')
  return (
    <Button
      type="button"
      size="icon-sm"
      variant="ghost"
      aria-label={t('snooze.label')}
      title={t('snooze.label')}
      onClick={() => {
        ids.forEach((id) => {
          snoozeNow(id, '1d')
        })
        toast.add({
          title: t('snooze.done', { count: ids.length }),
          type: 'info',
          actionProps: toastAction(t('snooze.undo'), () => {
            ids.forEach((id) => {
              unsnoozeNow(id)
            })
          }),
        })
      }}
    >
      <BellOffGlassIcon size="1.2em" />
    </Button>
  )
}

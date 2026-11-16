import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import { useQueueAction, type QueueAction } from '../../queries/buildQueue'

// One place for the queue state change and its error copy, shared by the
// controls, the pause dialog and the banner Resume action.
export function useQueueChange() {
  const { t } = useTranslation('deploys')
  const mutation = useQueueAction()
  const run = (action: QueueAction, reason?: string, onDone?: () => void) => {
    mutation.mutate(
      { action, reason },
      {
        onSuccess: () => {
          toast.add({ title: t(`buildQueue.done.${action}`), type: 'success' })
          onDone?.()
        },
        onError: (err) => {
          const conflict = err instanceof ApiError && err.status === 409
          toast.add({
            title: conflict
              ? t(`buildQueue.conflict.${action}`)
              : t('buildQueue.failed'),
            description: conflict ? undefined : err.message,
            type: conflict ? 'info' : 'error',
          })
          if (conflict) onDone?.()
        },
      },
    )
  }
  return { run, pending: mutation.isPending }
}

import { useTranslation } from 'react-i18next'
import { toast, toastAction } from '@/components/ui/toast'
import { isPendingApproval, useTriggerDeploy } from '../queries/deploys'

/**
 * Re-triggers a deploy of the app's current image tag, with toasts.
 * onViewApp, when given, adds a "view app" action to the success toast:
 * callers already on the app's own page (e.g. a retry button inside
 * DeployFailureSummaryCard) have no reason to pass it.
 */
export function useRedeployApp(
  name: string,
  image: string,
  options?: { onViewApp?: () => void },
) {
  const { t } = useTranslation('common')
  const triggerDeploy = useTriggerDeploy(name)
  return {
    isPending: triggerDeploy.isPending,
    redeploy: () => {
      triggerDeploy.mutate(
        { image },
        {
          onSuccess: (result) => {
            const awaitingApproval = isPendingApproval(result)
            toast.add({
              title: awaitingApproval
                ? `Redeploy of "${name}" is waiting for approval.`
                : `Redeploying "${name}".`,
              type: 'success',
              actionProps:
                !awaitingApproval && options?.onViewApp
                  ? toastAction(t('actions.viewApp'), options.onViewApp)
                  : undefined,
            })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        },
      )
    },
  }
}

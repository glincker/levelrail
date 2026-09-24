import { toast } from '@/components/ui/toast'
import { isPendingApproval, useTriggerDeploy } from '../queries/deploys'

/** Re-triggers a deploy of the app's current image tag, with toasts. */
export function useRedeployApp(name: string, image: string) {
  const triggerDeploy = useTriggerDeploy(name)
  return {
    isPending: triggerDeploy.isPending,
    redeploy: () => {
      triggerDeploy.mutate(
        { image },
        {
          onSuccess: (result) => {
            toast.add({
              title: isPendingApproval(result)
                ? `Redeploy of "${name}" is waiting for approval.`
                : `Redeploying "${name}".`,
              type: 'success',
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

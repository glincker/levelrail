import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import { useRestartApp } from '../queries/apps'
import {
  applyTriggerDeployResult,
  isPendingApproval,
  triggerDeploy,
} from '../queries/deploys'

/** Restart and redeploy for an app chosen at run time (command palette). */
export function usePaletteAppActions() {
  const queryClient = useQueryClient()
  const restart = useRestartApp()
  const redeploy = useMutation({
    mutationFn: ({ name, image }: { name: string; image: string }) =>
      triggerDeploy(name, { image }),
    onSuccess: (result, { name }) =>
      applyTriggerDeployResult(queryClient, name, result),
  })

  return {
    restartApp: (name: string) =>
      restart.mutate(name, {
        onSuccess: () =>
          toast.add({ title: `Restarting "${name}".`, type: 'success' }),
        onError: (error) => toast.add({ title: error.message, type: 'error' }),
      }),
    redeployApp: (name: string, image: string) =>
      redeploy.mutate(
        { name, image },
        {
          onSuccess: (result) =>
            toast.add({
              title: isPendingApproval(result)
                ? `Redeploy of "${name}" is waiting for approval.`
                : `Redeploying "${name}".`,
              type: 'success',
            }),
          onError: (error) =>
            toast.add({ title: error.message, type: 'error' }),
        },
      ),
  }
}

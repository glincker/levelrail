import { toast } from '@/components/ui/toast'
import { useRestartApp } from '../queries/apps'

// The reconciler matches running containers by deterministic name only
// (controller.go), so a health-check/resource save updates the database
// but the running container is untouched until a restart or redeploy.
// timeout: 0 keeps the action visible until the user acts on it.
export function useRestartRequiredToast() {
  const restartApp = useRestartApp()

  return function notifyRestartRequired(appName: string, savedMessage: string) {
    const id = toast.add({
      title: savedMessage,
      description: 'Restart the app to apply this change.',
      type: 'warning',
      timeout: 0,
      actionProps: {
        children: 'Restart now',
        onClick: () => {
          toast.close(id)
          // Same mutate/onSuccess/onError shape as RestartAppButton.
          restartApp.mutate(appName, {
            onSuccess: () => {
              toast.add({ title: `Restarting "${appName}".`, type: 'success' })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        },
      },
    })
  }
}

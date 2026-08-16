import { toast } from '@/components/ui/toast'
import { useRestartApp } from '../queries/apps'

// Closes the gap named in the reconciler's own doc comments (internal/
// reconcile/application/controller.go, ContainerName/InspectByName): the
// reconciler only ever compares desired state against a running
// container by its deterministic name, never by its actual env, health
// check, or resource config, so saving one of those fields updates the
// database but leaves the already-running container untouched until a
// restart (new RestartNonce) or a redeploy (new image) forces a new
// container name. A plain "Saved." toast after one of those saves would
// therefore be misleading, it implies the change already took effect.
//
// A transient toast, not a Coolify-style persistent "configuration
// changed" banner, is the right shape here: Coolify's banner
// (resources/views/livewire/project/shared/configuration-checker.blade.php
// in docs-local/competitor-clones/coolify) can stay pinned across page
// loads because it is computed from a stored config_hash diff against
// the last deployed snapshot, real, durable "is the running thing stale"
// state. This app's reconciler does not compute or expose that
// comparison anywhere (that absence is exactly the gap this hook papers
// over at the UX layer), so there is no persisted "still pending" fact
// to redraw a banner from after this toast is gone. What this hook does
// know, for certain, is that the save that just happened needs a
// restart; an actionable toast tied to that specific save event is
// honest about the scope of what it knows. Revisit as a persistent
// banner if the reconciler ever grows a real drift-detection endpoint.
//
// Deliberately timeout: 0 (no auto dismiss): the existing plain "saved"
// toasts (RestartAppButton, other editors) are fire-and-forget
// confirmations, fine to lose after a few seconds. This one carries an
// action the user came here to take; auto-dismissing it before they
// notice would recreate the exact silent-gap problem it exists to fix.
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
          // Same mutate/onSuccess/onError shape and copy as
          // RestartAppButton (components/RestartAppButton.tsx), so a
          // restart triggered from this toast behaves identically to one
          // triggered from the app detail page's own Restart button,
          // including how a "nothing to restart" failure surfaces:
          // RestartService has no precondition on a running container
          // existing, so there is no separate disabled state to invent
          // here, an error (if any) just comes back through onError like
          // any other restart failure.
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

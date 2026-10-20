import { LockKeyIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useExecAccess, useSetExecAccess } from '../queries/execAccess'

// ExecAccessCard is the opt-out toggle for shell/exec access (GET/PUT
// /api/v1/apps/{name}/exec-access): on by default, the opposite shape
// AutoRollbackCard's own "Enabled" toggle establishes for its opt-in
// feature. Turning this off blocks POST .../exec and the interactive
// terminal for this app even for a caller whose IAM abilities would
// otherwise allow it (internal/api/exec.go's requireExecAccess), so
// disabling it is a deliberate, separate step from ordinary app
// permissions. Rendered above AppTerminal/ExecPanel on the Exec route.
export function ExecAccessCard({ appName }: { appName: string }) {
  const setting = useExecAccess(appName)
  const setExecAccess = useSetExecAccess(appName)

  function toggle(next: boolean) {
    setExecAccess.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next
            ? 'Shell/exec access enabled.'
            : 'Shell/exec access disabled.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not update exec access.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockKeyIcon className="size-4 text-muted-foreground" />
          Shell/exec access
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">Enabled</p>
            <p className="text-sm text-muted-foreground">
              Whether the one-off command runner and the interactive terminal
              below are allowed to reach this app&apos;s container at all.
              Turning this off blocks both even for a token or session that
              otherwise has full access, useful for locking down a production
              app. Re-enabling it is an explicit, separate step.
            </p>
          </div>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setExecAccess.isPending}
            aria-label="Shell/exec access enabled"
          />
        </div>
      </CardContent>
    </Card>
  )
}

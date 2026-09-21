import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HelpLink } from '@/components/HelpLink'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useAutoRollback, useSetAutoRollback } from '../queries/autoRollback'

// AutoRollbackCard is the opt-in toggle for
// internal/alerting.MaybeAutoRollback (GET/PUT
// /api/v1/apps/{name}/auto-rollback): off by default, the same
// risky-by-default-feature-is-opt-in shape PreviewEnvironmentsCard's own
// "Enabled" toggle already establishes. Rendered above DeployAttemptsList
// on the Deploys route, since this setting only makes sense next to real
// deploy history.
export function AutoRollbackCard({ appName }: { appName: string }) {
  const setting = useAutoRollback(appName)
  const setAutoRollback = useSetAutoRollback(appName)

  function toggle(next: boolean) {
    setAutoRollback.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next
            ? 'Auto-rollback on crashloop enabled.'
            : 'Auto-rollback on crashloop disabled.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not update auto-rollback.',
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
          <ArrowCounterClockwiseIcon className="size-4 text-muted-foreground" />
          Auto-rollback on crashloop
          <HelpLink
            path="/observability#alert-rules"
            label="Auto-rollback guide"
          />
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">Enabled</p>
            <p className="text-sm text-muted-foreground">
              When a crashloop alert fires for this app (see the Alerts tab),
              automatically redeploy the most recent successful image older than
              the one that's crashlooping. Fires at most once per crashloop
              episode; if there's no older successful image to fall back to, the
              app is left to the crashloop alert alone.
            </p>
          </div>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setAutoRollback.isPending}
            aria-label="Auto-rollback on crashloop enabled"
          />
        </div>
      </CardContent>
    </Card>
  )
}

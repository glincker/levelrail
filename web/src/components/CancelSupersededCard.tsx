import { SkipForwardIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { InfoTip } from './kit'
import {
  useCancelSuperseded,
  useSetCancelSuperseded,
} from '../queries/deployControl'

// Per-app opt-in (GET/PUT /api/v1/apps/{name}/cancel-superseded): off by
// default so no queued deploy is ever dropped unless the operator asks.
export function CancelSupersededCard({ appName }: { appName: string }) {
  const setting = useCancelSuperseded(appName)
  const setSetting = useSetCancelSuperseded(appName)

  function toggle(next: boolean) {
    setSetting.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next
            ? 'Superseded queued deploys will be replaced.'
            : 'Queued deploys are kept.',
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not update the setting.',
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
          <SkipForwardIcon className="size-4 text-muted-foreground" />
          Replace superseded queued deploys
          <InfoTip label="About replacing queued deploys">
            Only deploys still waiting in the queue are replaced, and only by a
            newer one for the same branch. A deploy that already started
            building is never canceled automatically.
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">Enabled</p>
            <p className="text-sm text-muted-foreground">
              When a newer commit for a branch is queued behind a running
              deploy, older queued deploys for that branch are marked superseded
              instead of building one after another.
            </p>
          </div>
          <Switch
            checked={setting.data?.enabled ?? false}
            onCheckedChange={toggle}
            disabled={setting.isPending || setSetting.isPending}
            aria-label="Replace superseded queued deploys"
          />
        </div>
      </CardContent>
    </Card>
  )
}

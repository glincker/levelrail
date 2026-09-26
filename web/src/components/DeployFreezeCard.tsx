import { useState } from 'react'
import {
  PlusIcon,
  SnowflakeIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { HelpLink } from '@/components/HelpLink'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useDeployFreezeOptional,
  useSetDeployFreeze,
  type FreezeWindow,
} from '../queries/deployFreeze'

const EMPTY_WINDOW: FreezeWindow = {
  cron: '0 17 * * 5',
  duration: '64h',
  timezone: 'UTC',
  reason: '',
}

// DeployFreezeCard edits an app's deploy freeze windows. While a window is
// active, webhook and pipeline deploys are held and released when it ends;
// manual deploys need an explicit override with a reason.
export function DeployFreezeCard({ appName }: { appName: string }) {
  const freeze = useDeployFreezeOptional(appName)
  const save = useSetDeployFreeze(appName)
  const [draft, setDraft] = useState<FreezeWindow[] | null>(null)

  if (freeze.isError || !freeze.data) return null
  const windows = draft ?? freeze.data.windows
  const status = freeze.data.status

  const update = (i: number, patch: Partial<FreezeWindow>) => {
    setDraft(windows.map((w, j) => (j === i ? { ...w, ...patch } : w)))
  }

  const onSave = () => {
    save.mutate(windows, {
      onSuccess: () => {
        setDraft(null)
        toast.add({ title: 'Deploy freeze saved.', type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not save the deploy freeze.',
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
          <SnowflakeIcon className="size-4 text-muted-foreground" />
          Deploy freeze
          {status.frozen ? (
            <Badge variant="warning">
              Frozen
              {status.until
                ? ` until ${new Date(status.until).toLocaleString()}`
                : ''}
            </Badge>
          ) : (
            <Badge variant="muted">Open</Badge>
          )}
          <HelpLink
            path="/deploy-safety#freeze-windows"
            label="Deploy freeze guide"
          />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Each window starts at every match of its cron expression, in its
          timezone, and lasts its duration. Automatic deploys (git pushes,
          pipeline deploy steps) are held and run when the window ends. Manual
          deploys need an override with a reason, recorded on the deploy.
        </p>

        {windows.length === 0 ? (
          <p className="text-sm text-muted-foreground italic">
            No freeze windows for this app.
          </p>
        ) : (
          <ul className="space-y-3">
            {windows.map((w, i) => (
              <li
                key={w.id ?? `new-${i}`}
                className="grid grid-cols-1 gap-2 rounded-md border border-border p-3 sm:grid-cols-[1fr_7rem_10rem_1fr_auto] sm:items-end"
              >
                <Field>
                  <FieldLabel htmlFor={`freeze-cron-${i}`}>Cron</FieldLabel>
                  <Input
                    id={`freeze-cron-${i}`}
                    className="font-mono"
                    value={w.cron}
                    onChange={(e) => update(i, { cron: e.target.value })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`freeze-duration-${i}`}>
                    Duration
                  </FieldLabel>
                  <Input
                    id={`freeze-duration-${i}`}
                    className="font-mono"
                    value={w.duration}
                    onChange={(e) => update(i, { duration: e.target.value })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`freeze-tz-${i}`}>Timezone</FieldLabel>
                  <Input
                    id={`freeze-tz-${i}`}
                    value={w.timezone ?? ''}
                    placeholder="UTC"
                    onChange={(e) => update(i, { timezone: e.target.value })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={`freeze-reason-${i}`}>Reason</FieldLabel>
                  <Input
                    id={`freeze-reason-${i}`}
                    value={w.reason ?? ''}
                    onChange={(e) => update(i, { reason: e.target.value })}
                  />
                </Field>
                <Button
                  variant="outline"
                  size="sm"
                  aria-label={`Remove freeze window ${i + 1}`}
                  onClick={() => setDraft(windows.filter((_, j) => j !== i))}
                >
                  <TrashIcon className="size-3.5" />
                </Button>
              </li>
            ))}
          </ul>
        )}

        {freeze.data.inherited && freeze.data.inherited.length > 0 ? (
          <div className="text-sm text-muted-foreground">
            <p className="font-medium text-foreground">
              Inherited from the global freeze
            </p>
            <ul className="mt-1 space-y-1">
              {freeze.data.inherited.map((w) => (
                <li key={w.id} className="font-mono text-xs">
                  {w.cron} for {w.duration} ({w.timezone || 'UTC'})
                  {w.reason ? ` · ${w.reason}` : ''}
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setDraft([...windows, { ...EMPTY_WINDOW }])}
          >
            <PlusIcon className="size-3.5" data-icon="inline-start" />
            Add window
          </Button>
          <Button
            size="sm"
            onClick={onSave}
            disabled={draft === null || save.isPending}
          >
            {save.isPending ? 'Saving...' : 'Save'}
          </Button>
          {draft !== null ? (
            <Button variant="ghost" size="sm" onClick={() => setDraft(null)}>
              Discard changes
            </Button>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

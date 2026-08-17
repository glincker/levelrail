import { useState } from 'react'
import {
  ArrowSquareOutIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useClearLogDrain, useSetLogDrain } from '../queries/apps'
import type { AppDetail, LogDrainType } from '../types/appDetail'

// Configure/remove card for PUT/DELETE /api/v1/apps/{name}/log-drain
// (internal/api/apps_log_drain.go): forwards this app's container logs
// to an external HTTP or syslog sink, additive to the built-in
// node-local log store (Logs tab), never a replacement for it.
export function LogDrainCard({ app }: { app: AppDetail }) {
  const existing = app.log_drain
  const [type, setType] = useState<LogDrainType>(existing?.type ?? 'http')
  const [target, setTarget] = useState(existing?.target ?? '')
  const setLogDrain = useSetLogDrain()
  const clearLogDrain = useClearLogDrain()
  const pending = setLogDrain.isPending || clearLogDrain.isPending

  function handleSave(enabled: boolean) {
    if (type === 'http' && !target.trim()) {
      toast.add({
        title: 'Target required.',
        description: 'An HTTP drain needs a URL to POST log lines to.',
        type: 'error',
      })
      return
    }
    setLogDrain.mutate(
      { name: app.name, drain: { type, target: target.trim(), enabled } },
      {
        onSuccess: () => {
          toast.add({
            title: enabled ? 'Log drain saved.' : 'Log drain paused.',
            description: `${app.name}'s logs ${enabled ? 'now forward to' : 'are configured for, but not currently sent to,'} this sink.`,
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not save log drain.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleRemove() {
    clearLogDrain.mutate(app.name, {
      onSuccess: () => {
        setTarget('')
      },
      onError: (error) => {
        toast.add({
          title: 'Could not remove log drain.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Log drain</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Forward this app&apos;s container logs to an external HTTP endpoint or
          syslog server, in addition to the built-in log store above.
        </p>

        {existing ? (
          <div className="flex items-center justify-between gap-4 rounded-lg border border-input bg-muted/50 p-3">
            <div className="flex min-w-0 items-start gap-2">
              <PlugsConnectedIcon
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <div className="min-w-0">
                <p className="text-sm font-medium text-foreground">
                  {existing.type === 'http' ? 'HTTP webhook' : 'Syslog'}
                </p>
                <p className="truncate text-sm text-muted-foreground">
                  {existing.target || 'local syslog daemon'}
                </p>
              </div>
            </div>
            <Switch
              checked={existing.enabled}
              onCheckedChange={(checked) => {
                setType(existing.type)
                setTarget(existing.target)
                setLogDrain.mutate(
                  {
                    name: app.name,
                    drain: { ...existing, enabled: checked },
                  },
                  {
                    onError: (error) => {
                      toast.add({
                        title: 'Could not update log drain.',
                        description: error.message,
                        type: 'error',
                      })
                    },
                  },
                )
              }}
              disabled={pending}
              aria-label="Log drain enabled"
            />
          </div>
        ) : null}

        <div className="space-y-3">
          <Field>
            <FieldLabel htmlFor="log-drain-type">Sink type</FieldLabel>
            <Select
              value={type}
              onValueChange={(value) => {
                if (value) {
                  setType(value)
                }
              }}
            >
              <SelectTrigger id="log-drain-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="http">HTTP webhook</SelectItem>
                <SelectItem value="syslog">Syslog</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="log-drain-target">
              {type === 'http' ? 'Endpoint URL' : 'Syslog address'}
            </FieldLabel>
            <Input
              id="log-drain-target"
              placeholder={
                type === 'http'
                  ? 'https://collector.example.com/ingest'
                  : 'udp://logs.example.com:514 (blank for local)'
              }
              value={target}
              onChange={(e) => {
                setTarget(e.target.value)
              }}
              disabled={pending}
            />
            <FieldDescription>
              {type === 'http'
                ? 'Each batch of log lines is POSTed here as a JSON array.'
                : 'network://host:port, e.g. udp://logs.example.com:514. Leave blank to write to this node’s local syslog daemon.'}
            </FieldDescription>
          </Field>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              size="sm"
              disabled={pending}
              onClick={() => handleSave(true)}
            >
              <ArrowSquareOutIcon className="size-3.5" aria-hidden="true" />
              {setLogDrain.isPending
                ? 'Saving...'
                : existing
                  ? 'Update'
                  : 'Save'}
            </Button>
            {existing ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={pending}
                onClick={handleRemove}
              >
                {clearLogDrain.isPending ? 'Removing...' : 'Remove'}
              </Button>
            ) : null}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

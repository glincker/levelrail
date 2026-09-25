import { useState } from 'react'
import { ArchiveIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import {
  useDeleteLogArchivePolicy,
  useLogArchivePolicies,
  useSetLogArchivePolicy,
  useStorageDestinationsOptional,
} from '../queries/storage'
import { LogArchiveDumpAndRuns } from './LogArchiveDumpAndRuns'
import { LogArchiveObjects } from './LogArchiveObjects'
import type { LogArchivePolicy, StorageDestination } from '../types/storage'

const INTERVALS = ['15m', '30m', '1h', '6h', '12h', '24h']

function normalizeInterval(interval: string): string {
  const match = /^(\d+)h(\d+)m/.exec(interval)
  if (!match) return interval
  const hours = Number(match[1])
  const minutes = Number(match[2])
  if (hours > 0 && minutes === 0) return `${hours}h`
  if (hours === 0) return `${minutes}m`
  return interval
}

function PolicyForm({
  appName,
  destinations,
  policy,
}: {
  appName: string
  destinations: StorageDestination[]
  policy: LogArchivePolicy | undefined
}) {
  const [target, setTarget] = useState(
    policy?.target_id ?? destinations[0]?.id ?? '',
  )
  const [interval, setInterval] = useState(
    policy ? normalizeInterval(policy.interval) : '1h',
  )
  const [retention, setRetention] = useState(
    String(policy?.retention_days ?? 30),
  )
  const [enabled, setEnabled] = useState(policy?.enabled ?? true)
  const save = useSetLogArchivePolicy()
  const remove = useDeleteLogArchivePolicy()
  const retentionDays = Number.parseInt(retention, 10)
  const retentionValid = Number.isInteger(retentionDays) && retentionDays >= 0

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (target === '' || !retentionValid) return
        save.mutate(
          {
            app_name: appName,
            target_id: target,
            interval,
            retention_days: retentionDays,
            enabled,
          },
          {
            onSuccess: () => {
              toast.add({ title: 'Log archive policy saved.', type: 'success' })
            },
          },
        )
      }}
    >
      <div className="grid gap-4 sm:grid-cols-3">
        <Field>
          <FieldLabel htmlFor={`archive-target-${appName}`}>
            Destination
          </FieldLabel>
          <Select
            value={target}
            onValueChange={(v) => {
              if (v !== null) setTarget(v)
            }}
          >
            <SelectTrigger id={`archive-target-${appName}`} className="w-full">
              <SelectValue placeholder="Choose a destination" />
            </SelectTrigger>
            <SelectContent>
              {destinations.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {d.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`archive-interval-${appName}`}>
            Ship every
          </FieldLabel>
          <Select
            value={interval}
            onValueChange={(v) => {
              if (v !== null) setInterval(v)
            }}
          >
            <SelectTrigger
              id={`archive-interval-${appName}`}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(INTERVALS.includes(interval)
                ? INTERVALS
                : [interval, ...INTERVALS]
              ).map((i) => (
                <SelectItem key={i} value={i}>
                  {i}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`archive-retention-${appName}`}>
            Keep for (days)
          </FieldLabel>
          <Input
            id={`archive-retention-${appName}`}
            inputMode="numeric"
            value={retention}
            onChange={(e) => {
              setRetention(e.target.value)
            }}
          />
          <FieldDescription>0 keeps objects forever.</FieldDescription>
        </Field>
      </div>
      <div className="flex items-center gap-2 text-sm">
        <Switch
          id={`archive-enabled-${appName}`}
          checked={enabled}
          onCheckedChange={setEnabled}
        />
        <label htmlFor={`archive-enabled-${appName}`}>Enabled</label>
      </div>
      {policy?.last_error ? (
        <Alert variant="destructive">
          <AlertTitle>Last run failed</AlertTitle>
          <AlertDescription>{policy.last_error}</AlertDescription>
        </Alert>
      ) : policy?.last_success_at ? (
        <p className="text-sm text-muted-foreground">
          Last successful archive:{' '}
          {new Date(policy.last_success_at).toLocaleString()}
        </p>
      ) : null}
      {save.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      ) : null}
      <div className="flex gap-2">
        <Button
          type="submit"
          disabled={save.isPending || target === '' || !retentionValid}
        >
          {policy ? 'Update policy' : 'Start archiving'}
        </Button>
        {policy ? (
          <Button
            type="button"
            variant="outline"
            disabled={remove.isPending}
            onClick={() => {
              remove.mutate(appName, {
                onSuccess: () => {
                  toast.add({
                    title: 'Log archive policy removed.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            Stop archiving
          </Button>
        ) : null}
      </div>
    </form>
  )
}

// Archive controls for one app (appName) or every app (appName ''):
// schedule policy, dump-now, run history, and archived object browser.
export function LogArchivePanel({ appName }: { appName: string }) {
  const destinations = useStorageDestinationsOptional()
  const policies = useLogArchivePolicies()
  const scope = appName === '' ? 'all apps' : appName

  if (destinations.isError || policies.isError) {
    const error = destinations.error ?? policies.error
    const notConfigured = error instanceof ApiError && error.status === 501
    return (
      <Alert>
        <AlertTitle>
          {notConfigured
            ? 'Log archive is not configured on this server'
            : 'Could not load log archive settings'}
        </AlertTitle>
        <AlertDescription>
          {notConfigured
            ? 'Set APP_MASTER_KEY and restart the control plane to connect object storage.'
            : (error?.message ?? 'Unknown error')}
        </AlertDescription>
      </Alert>
    )
  }
  if (destinations.isPending || policies.isPending) {
    return <p className="text-sm text-muted-foreground">Loading...</p>
  }

  const policy = policies.data.find((p) => p.app_name === appName)
  const dests = destinations.data

  if (dests.length === 0) {
    return (
      <Alert>
        <AlertTitle>No storage destination yet</AlertTitle>
        <AlertDescription>
          Connect a bucket under Settings, Storage destinations, then archive
          logs for {scope} here.
        </AlertDescription>
      </Alert>
    )
  }

  const objectsTarget = policy?.target_id ?? dests[0]?.id ?? ''

  return (
    <div className="space-y-8">
      <section aria-labelledby={`archive-policy-${appName}`}>
        <h2
          id={`archive-policy-${appName}`}
          className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground"
        >
          <ArchiveIcon className="size-4" aria-hidden="true" />
          Archive schedule for {scope}
        </h2>
        <PolicyForm
          key={policy?.id ?? 'new'}
          appName={appName}
          destinations={dests}
          policy={policy}
        />
      </section>
      <LogArchiveDumpAndRuns
        appName={appName}
        destinations={dests}
        defaultTarget={objectsTarget}
      />
      <LogArchiveObjects appName={appName} targetId={objectsTarget} />
    </div>
  )
}

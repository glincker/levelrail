import { useState } from 'react'
import { ClockCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useLogArchiveRuns, useStartLogArchiveDump } from '../queries/storage'
import type { LogArchiveRun, StorageDestination } from '../types/storage'

function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`
}

function RunRow({ run }: { run: LogArchiveRun }) {
  const variant =
    run.status === 'failed'
      ? 'destructive'
      : run.status === 'running'
        ? 'warning'
        : 'success'
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border py-2 text-sm last:border-b-0">
      <Badge variant={variant}>{run.status}</Badge>
      <span className="text-muted-foreground">{run.kind}</span>
      <span className="text-foreground">
        {run.app_name === '' ? 'all apps' : run.app_name}
      </span>
      <span className="text-muted-foreground">
        {run.lines} lines, {run.objects} objects, {formatBytes(run.bytes)}
      </span>
      <span className="text-xs text-muted-foreground">
        {new Date(run.started_at).toLocaleString()}
      </span>
      {run.error ? (
        <span className="w-full text-xs text-destructive">{run.error}</span>
      ) : null}
    </li>
  )
}

// Manual "dump now" for a time range plus the recent run history.
export function LogArchiveDumpAndRuns({
  appName,
  destinations,
  defaultTarget,
}: {
  appName: string
  destinations: StorageDestination[]
  defaultTarget: string
}) {
  const [target, setTarget] = useState(defaultTarget)
  const [from, setFrom] = useState(() =>
    toLocalInput(new Date(Date.now() - 24 * 3600 * 1000)),
  )
  const [to, setTo] = useState(() => toLocalInput(new Date()))
  const dump = useStartLogArchiveDump(appName)
  const runs = useLogArchiveRuns(appName)
  const valid = target !== '' && from !== '' && to !== '' && from < to

  return (
    <section aria-labelledby={`archive-dump-${appName}`} className="space-y-3">
      <h2
        id={`archive-dump-${appName}`}
        className="flex items-center gap-2 text-sm font-semibold text-foreground"
      >
        <ClockCounterClockwiseIcon className="size-4" aria-hidden="true" />
        Dump a time range now
      </h2>
      <form
        className="grid gap-3 sm:grid-cols-4 sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          if (!valid) return
          dump.mutate(
            {
              app_name: appName,
              target_id: target,
              from: new Date(from).toISOString(),
              to: new Date(to).toISOString(),
            },
            {
              onSuccess: () => {
                toast.add({ title: 'Dump started.', type: 'success' })
              },
            },
          )
        }}
      >
        <Field>
          <FieldLabel htmlFor={`dump-target-${appName}`}>
            Destination
          </FieldLabel>
          <Select
            value={target}
            onValueChange={(v) => {
              if (v !== null) setTarget(v)
            }}
          >
            <SelectTrigger id={`dump-target-${appName}`} className="w-full">
              <SelectValue />
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
          <FieldLabel htmlFor={`dump-from-${appName}`}>From</FieldLabel>
          <Input
            id={`dump-from-${appName}`}
            type="datetime-local"
            value={from}
            onChange={(e) => {
              setFrom(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`dump-to-${appName}`}>To</FieldLabel>
          <Input
            id={`dump-to-${appName}`}
            type="datetime-local"
            value={to}
            onChange={(e) => {
              setTo(e.target.value)
            }}
          />
        </Field>
        <Button type="submit" disabled={!valid || dump.isPending}>
          {dump.isPending ? 'Starting...' : 'Dump now'}
        </Button>
      </form>
      {dump.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {dump.error.message}
        </p>
      ) : null}
      {runs.data && runs.data.length > 0 ? (
        <ul aria-label="Recent archive runs">
          {runs.data.slice(0, 8).map((r) => (
            <RunRow key={r.id} run={r} />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No archive runs yet.</p>
      )}
    </section>
  )
}

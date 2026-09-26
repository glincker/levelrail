import { useState } from 'react'
import {
  CalendarBlankIcon,
  PlusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toast } from '@/components/ui/toast'
import {
  useDeleteMaintenanceWindow,
  useMaintenanceWindows,
  useSaveMaintenanceWindow,
} from '../queries/alertNoise'
import type { MaintenanceScope, MaintenanceWindow } from '../types/alertNoise'

const SCOPES: MaintenanceScope[] = ['all', 'app', 'node']

function browserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

function WindowDialog({
  existing,
  trigger,
  label,
}: {
  existing?: MaintenanceWindow
  trigger: React.ReactElement
  label: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(existing?.name ?? '')
  const [cron, setCron] = useState(existing?.cron ?? '0 3 * * 0')
  const [duration, setDuration] = useState(existing?.duration ?? '2h')
  const [timezone, setTimezone] = useState(
    existing?.timezone ?? browserTimezone(),
  )
  const [scope, setScope] = useState<MaintenanceScope>(existing?.scope ?? 'all')
  const [targets, setTargets] = useState((existing?.targets ?? []).join(', '))
  const [enabled, setEnabled] = useState(existing?.enabled ?? true)
  const save = useSaveMaintenanceWindow()

  function submit() {
    save.mutate(
      {
        id: existing?.id,
        name: name.trim(),
        cron: cron.trim(),
        duration: duration.trim(),
        timezone: timezone.trim(),
        scope,
        targets: targets
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
        enabled,
      },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({ title: 'Maintenance window saved.', type: 'success' })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger}>{label}</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {existing ? 'Edit maintenance window' : 'New maintenance window'}
          </DialogTitle>
          <DialogDescription>
            A recurring silence: it starts at each cron match in the chosen
            timezone and lasts the given duration. Alerts still evaluate and are
            recorded, they just do not notify.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Field>
            <FieldLabel htmlFor="mw-name">Name</FieldLabel>
            <Input
              id="mw-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mw-cron">Starts (cron)</FieldLabel>
            <Input
              id="mw-cron"
              className="font-mono"
              value={cron}
              onChange={(e) => {
                setCron(e.target.value)
              }}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="mw-duration">Duration</FieldLabel>
              <Input
                id="mw-duration"
                value={duration}
                onChange={(e) => {
                  setDuration(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="mw-tz">Timezone</FieldLabel>
              <Input
                id="mw-tz"
                value={timezone}
                onChange={(e) => {
                  setTimezone(e.target.value)
                }}
              />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="mw-scope">Applies to</FieldLabel>
            <Select
              value={scope}
              onValueChange={(v) => {
                setScope(v as MaintenanceScope)
              }}
            >
              <SelectTrigger id="mw-scope" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SCOPES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s === 'all' ? 'all alerts' : `specific ${s}s`}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          {scope !== 'all' ? (
            <Field>
              <FieldLabel htmlFor="mw-targets">
                {scope === 'app' ? 'App names' : 'Node names'}
              </FieldLabel>
              <Input
                id="mw-targets"
                placeholder="comma separated"
                value={targets}
                onChange={(e) => {
                  setTargets(e.target.value)
                }}
              />
            </Field>
          ) : null}
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={enabled} onCheckedChange={setEnabled} />
            Enabled
          </label>
          {save.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {save.error.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button type="button" disabled={save.isPending} onClick={submit}>
            {save.isPending ? 'Saving...' : 'Save window'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function WindowRow({ window: w }: { window: MaintenanceWindow }) {
  const del = useDeleteMaintenanceWindow()
  const state = !w.enabled ? 'disabled' : w.active ? 'active' : 'idle'
  return (
    <TableRow>
      <TableCell className="font-medium text-foreground">{w.name}</TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {w.cron} ({w.timezone ?? 'UTC'}) for {w.duration}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {w.scope === 'all'
          ? 'all alerts'
          : `${w.scope}: ${(w.targets ?? []).join(', ')}`}
      </TableCell>
      <TableCell>
        <Badge
          variant={
            state === 'active'
              ? 'warning'
              : state === 'idle'
                ? 'outline'
                : 'muted'
          }
        >
          {state}
        </Badge>
      </TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground">
        {w.next_start ? new Date(w.next_start).toLocaleString() : '-'}
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-2">
          <WindowDialog
            existing={w}
            trigger={<Button type="button" variant="outline" size="sm" />}
            label="Edit"
          />
          <Button
            type="button"
            variant="destructive"
            size="sm"
            disabled={del.isPending}
            onClick={() => {
              if (w.id) {
                del.mutate(w.id, {
                  onSuccess: () => {
                    toast.add({ title: 'Window deleted.', type: 'success' })
                  },
                })
              }
            }}
          >
            Delete
          </Button>
        </div>
      </TableCell>
    </TableRow>
  )
}

export function MaintenanceWindowsPanel() {
  const { data, isLoading, error } = useMaintenanceWindows()
  const windows = data ?? []
  const createLabel = (
    <>
      <PlusCircleIcon className="size-4" aria-hidden="true" />
      New window
    </>
  )

  return (
    <section className="space-y-3" aria-label="Maintenance windows">
      <div className="flex justify-end">
        <WindowDialog
          trigger={<Button size="sm" type="button" />}
          label={createLabel}
        />
      </div>
      {isLoading ? (
        <TableSkeleton columnCount={6} rowCount={3} />
      ) : error ? (
        <p className="text-sm text-destructive">{error.message}</p>
      ) : windows.length === 0 ? (
        <EmptyState
          icon={<CalendarBlankIcon className="size-5" />}
          title="No maintenance windows"
          description="Schedule a recurring window so planned work does not page anyone."
        />
      ) : (
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Schedule</TableHead>
                <TableHead>Applies to</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Next start</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {windows.map((w) => (
                <WindowRow key={w.id} window={w} />
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}

import { useState } from 'react'
import { Button } from '@/components/ui/button'
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
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { parseRecipients, recipientProblem } from '../lib/controlPlaneDr'
import { useStorageDestinationsOptional } from '../queries/storage'
import {
  useUpdateControlPlaneDr,
  type ControlPlaneDr,
} from '../queries/controlPlaneDr'

const NONE = '__none__'

function NumberField({
  id,
  label,
  value,
  onChange,
}: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        type="number"
        min={0}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
    </Field>
  )
}

export function ControlPlaneDrSettings({ status }: { status: ControlPlaneDr }) {
  const destinations = useStorageDestinationsOptional().data ?? []
  const update = useUpdateControlPlaneDr()
  const [enabled, setEnabled] = useState(status.enabled)
  const [target, setTarget] = useState(status.target_id)
  const [escrowTarget, setEscrowTarget] = useState(status.escrow_target_id)
  const [recipients, setRecipients] = useState(status.recipients.join('\n'))
  const [schedule, setSchedule] = useState(status.schedule)
  const [drillSchedule, setDrillSchedule] = useState(status.drill_schedule)
  const [daily, setDaily] = useState(String(status.retain_daily))
  const [weekly, setWeekly] = useState(String(status.retain_weekly))
  const [monthly, setMonthly] = useState(String(status.retain_monthly))

  const parsed = parseRecipients(recipients)
  const problem = recipientProblem(parsed)
  const backupDest = destinations.find((d) => d.id === target)
  const escrowDest = destinations.find((d) => d.id === escrowTarget)
  const sameBucket =
    backupDest !== undefined &&
    escrowDest !== undefined &&
    (backupDest.id === escrowDest.id ||
      (backupDest.bucket === escrowDest.bucket &&
        (backupDest.endpoint ?? '') === (escrowDest.endpoint ?? '')))

  const destSelect = (
    id: string,
    value: string,
    onChange: (v: string) => void,
    allowNone: boolean,
  ) => (
    <Select
      value={value === '' ? NONE : value}
      onValueChange={(v) => {
        if (v !== null) onChange(v === NONE ? '' : v)
      }}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {allowNone ? <SelectItem value={NONE}>None</SelectItem> : null}
        {destinations.map((d) => (
          <SelectItem key={d.id} value={d.id}>
            {d.name} ({d.bucket})
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (problem !== null) return
        update.mutate(
          {
            enabled,
            target_id: target,
            recipients: parsed,
            schedule,
            drill_schedule: drillSchedule,
            retain_daily: Number(daily) || 0,
            retain_weekly: Number(weekly) || 0,
            retain_monthly: Number(monthly) || 0,
            escrow_target_id: escrowTarget,
          },
          {
            onSuccess: () => {
              toast.add({ title: 'Disaster recovery saved.', type: 'success' })
            },
            onError: (err) => {
              toast.add({ title: err.message, type: 'error' })
            },
          },
        )
      }}
    >
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="text-sm font-medium text-foreground">
            Encrypted off-box backups
          </p>
          <p className="text-xs text-muted-foreground">
            Each backup is encrypted to your public keys before it leaves this
            server.
          </p>
        </div>
        <Switch
          aria-label="Enable encrypted off-box backups"
          checked={enabled}
          onCheckedChange={setEnabled}
        />
      </div>

      <Field>
        <FieldLabel htmlFor="cpdr-target">Backup destination</FieldLabel>
        {destinations.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No storage destination yet. Add an S3 compatible bucket under{' '}
            <a className="underline" href="/settings/storage">
              Settings, Storage
            </a>{' '}
            first.
          </p>
        ) : (
          destSelect('cpdr-target', target, setTarget, true)
        )}
      </Field>

      <Field>
        <FieldLabel htmlFor="cpdr-recipients">
          Recipients (age public keys, one per line)
        </FieldLabel>
        <Textarea
          id="cpdr-recipients"
          className="font-mono text-xs"
          placeholder="age1..."
          value={recipients}
          onChange={(e) => {
            setRecipients(e.target.value)
          }}
          aria-invalid={problem !== null}
        />
        {problem !== null ? (
          <p className="text-sm text-destructive">{problem}</p>
        ) : (
          <p className="text-xs text-muted-foreground">
            Public keys only. Generate a pair with{' '}
            <span className="font-mono">
              levelrail-cli control-plane-backups keys generate
            </span>{' '}
            and keep the private key offline. Add a second person&apos;s key so
            either can restore.
          </p>
        )}
      </Field>

      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="cpdr-schedule">
            Backup schedule (cron)
          </FieldLabel>
          <Input
            id="cpdr-schedule"
            className="font-mono"
            value={schedule}
            onChange={(e) => {
              setSchedule(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="cpdr-drill">Restore drill (cron)</FieldLabel>
          <Input
            id="cpdr-drill"
            className="font-mono"
            value={drillSchedule}
            onChange={(e) => {
              setDrillSchedule(e.target.value)
            }}
          />
        </Field>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <NumberField
          id="cpdr-daily"
          label="Keep daily"
          value={daily}
          onChange={setDaily}
        />
        <NumberField
          id="cpdr-weekly"
          label="Keep weekly"
          value={weekly}
          onChange={setWeekly}
        />
        <NumberField
          id="cpdr-monthly"
          label="Keep monthly"
          value={monthly}
          onChange={setMonthly}
        />
      </div>

      {destinations.length > 0 ? (
        <Field>
          <FieldLabel htmlFor="cpdr-escrow-target">
            Escrow upload destination (optional)
          </FieldLabel>
          {destSelect(
            'cpdr-escrow-target',
            escrowTarget,
            setEscrowTarget,
            true,
          )}
          <p className="text-xs text-muted-foreground">
            Escrow is never uploaded unless you ask for it when generating the
            bundle. Keep it out of the backup bucket.
          </p>
          {sameBucket ? (
            <p role="alert" className="text-sm text-destructive">
              This is the same bucket as your backups. Whoever can read that
              bucket would get both the backup and the key to it, which defeats
              the point of escrow. Pick a different destination or leave this
              empty and store the bundle offline.
            </p>
          ) : null}
        </Field>
      ) : null}

      <Button type="submit" disabled={update.isPending || problem !== null}>
        {update.isPending ? 'Saving...' : 'Save'}
      </Button>
    </form>
  )
}

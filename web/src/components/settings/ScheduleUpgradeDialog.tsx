import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { Checkbox } from '../ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog'
import { Field, FieldLabel } from '../ui/field'
import { Input } from '../ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { formatInZone } from '../../lib/formatInZone'
import type { SelfUpgradeBreaking } from '../../queries/selfUpgrade'
import {
  ScheduleError,
  useCreateSchedule,
  type UpgradeWindow,
} from '../../queries/upgradeSchedule'

type When = 'at' | 'window'

export interface ScheduleUpgradeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  target: string
  breaking: SelfUpgradeBreaking[]
  serverTimezone: string
  nextWindow: UpgradeWindow | null
  replacing: boolean
}

export function ScheduleUpgradeDialog(props: ScheduleUpgradeDialogProps) {
  const { open, onOpenChange, target, breaking, serverTimezone, nextWindow } =
    props
  const { t } = useTranslation('updates')
  const create = useCreateSchedule()
  const [when, setWhen] = useState<When>('at')
  const [local, setLocal] = useState('')
  const [acked, setAcked] = useState<Record<string, boolean>>({})
  const [major, setMajor] = useState(false)

  const needAck = breaking.filter((b) => b.requires_ack)
  const allAcked = needAck.every((b) => acked[b.id])
  const majorRequired =
    create.error instanceof ScheduleError && create.error.needsMajorAck
  const canSubmit =
    allAcked &&
    (!majorRequired || major) &&
    (when === 'window' ? nextWindow !== null : local !== '') &&
    !create.isPending

  const submit = () => {
    create.mutate(
      {
        target,
        ack: needAck.filter((b) => acked[b.id]).map((b) => b.id),
        acknowledge_major: major,
        replace: props.replacing,
        ...(when === 'window'
          ? { window: true }
          : { at_local: local, timezone: serverTimezone }),
      },
      { onSuccess: () => onOpenChange(false) },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('schedule.dialogTitle', { version: target })}
          </DialogTitle>
          <DialogDescription>
            {t('schedule.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="schedule-when">
              {t('schedule.whenLabel')}
            </FieldLabel>
            <Select
              value={when}
              onValueChange={(v) => {
                if (v === 'at' || v === 'window') setWhen(v)
              }}
            >
              <SelectTrigger id="schedule-when" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="at">{t('schedule.whenAt')}</SelectItem>
                <SelectItem value="window" disabled={nextWindow === null}>
                  {t('schedule.whenWindow')}
                </SelectItem>
              </SelectContent>
            </Select>
          </Field>
          {when === 'at' ? (
            <Field>
              <FieldLabel htmlFor="schedule-at">
                {t('schedule.atLabel', { timezone: serverTimezone })}
              </FieldLabel>
              <Input
                id="schedule-at"
                type="datetime-local"
                value={local}
                onChange={(e) => setLocal(e.target.value)}
              />
            </Field>
          ) : nextWindow ? (
            <p className="text-sm text-muted-foreground">
              {t('schedule.windowNext', {
                time: formatInZone(nextWindow.start, serverTimezone),
                timezone: serverTimezone,
              })}
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('schedule.noWindow')}
            </p>
          )}
          {needAck.length > 0 ? (
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {t('selfUpgrade.breakingTitle')}
              </p>
              <ul className="space-y-2">
                {needAck.map((b) => (
                  <li
                    key={b.id}
                    className="flex items-start gap-2 rounded-md bg-muted px-3 py-2 text-sm"
                  >
                    <Checkbox
                      id={`sched-ack-${b.id}`}
                      checked={acked[b.id] === true}
                      onCheckedChange={(v) =>
                        setAcked((prev) => ({ ...prev, [b.id]: v === true }))
                      }
                    />
                    <label
                      htmlFor={`sched-ack-${b.id}`}
                      className="min-w-0 flex-1"
                    >
                      {b.summary}
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {majorRequired ? (
            <div className="flex items-start gap-2 rounded-md bg-muted px-3 py-2 text-sm">
              <Checkbox
                id="sched-major"
                checked={major}
                onCheckedChange={(v) => setMajor(v === true)}
              />
              <label htmlFor="sched-major">{t('schedule.majorAck')}</label>
            </div>
          ) : null}
          <p className="text-xs text-muted-foreground">
            {t('schedule.firesNote')}
          </p>
          {create.isError && !majorRequired ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('schedule.cancel')}
          </Button>
          <Button type="button" disabled={!canSubmit} onClick={submit}>
            {create.isPending
              ? t('schedule.saving')
              : props.replacing
                ? t('schedule.reschedule')
                : t('schedule.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

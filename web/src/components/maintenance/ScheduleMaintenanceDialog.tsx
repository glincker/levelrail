import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  useCreateScheduledMaintenance,
  type MaintenanceScope,
} from '../../queries/scheduledMaintenance'
import {
  browserTimeZone,
  fromLocalInputValue,
  toLocalInputValue,
} from '../../lib/zonedTime'

const HOUR_MS = 3_600_000
const SCOPES: MaintenanceScope[] = ['all', 'project', 'app']

function defaultStart(zone: string): string {
  return toLocalInputValue(Date.now() + 24 * HOUR_MS, zone)
}

export function ScheduleMaintenanceDialog() {
  const { t } = useTranslation('maintenance')
  const zone = browserTimeZone()
  const create = useCreateScheduledMaintenance()
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [message, setMessage] = useState('')
  const [scope, setScope] = useState<MaintenanceScope>('all')
  const [target, setTarget] = useState('')
  const [start, setStart] = useState(() => defaultStart(zone))
  const [end, setEnd] = useState(() =>
    toLocalInputValue(Date.now() + 25 * HOUR_MS, zone),
  )
  const [freeze, setFreeze] = useState(false)
  const [lead, setLead] = useState('60')
  const [formError, setFormError] = useState<string | null>(null)

  function reset() {
    setTitle('')
    setMessage('')
    setScope('all')
    setTarget('')
    setStart(defaultStart(zone))
    setEnd(toLocalInputValue(Date.now() + 25 * HOUR_MS, zone))
    setFreeze(false)
    setLead('60')
    setFormError(null)
    create.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const startsAt = fromLocalInputValue(start, zone)
    const endsAt = fromLocalInputValue(end, zone)
    if (Number.isNaN(startsAt) || Number.isNaN(endsAt) || endsAt <= startsAt) {
      setFormError(t('form.badRange'))
      return
    }
    setFormError(null)
    const leadMinutes = Math.max(0, Math.round(Number(lead) || 0))
    create.mutate(
      {
        title: title.trim(),
        ...(message.trim() ? { message: message.trim() } : {}),
        scope,
        ...(scope === 'all' ? {} : { target: target.trim() }),
        starts_at: new Date(startsAt).toISOString(),
        ends_at: new Date(endsAt).toISOString(),
        freeze,
        announce_lead_minutes: leadMinutes,
      },
      {
        onSuccess: () => {
          setOpen(false)
          reset()
        },
      },
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button />}>
        <PlusGlassIcon className="size-5" aria-hidden="true" />
        {t('schedule')}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('form.title')}</DialogTitle>
          <DialogDescription>
            {t('form.description', { zone })}
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="mw-title">{t('form.name')}</FieldLabel>
            <Input
              id="mw-title"
              required
              maxLength={120}
              value={title}
              placeholder={t('form.namePlaceholder')}
              onChange={(e) => {
                setTitle(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="mw-message">{t('form.message')}</FieldLabel>
            <Input
              id="mw-message"
              value={message}
              onChange={(e) => {
                setMessage(e.target.value)
              }}
            />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="mw-start">{t('form.start')}</FieldLabel>
              <Input
                id="mw-start"
                type="datetime-local"
                required
                value={start}
                onChange={(e) => {
                  setStart(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="mw-end">{t('form.end')}</FieldLabel>
              <Input
                id="mw-end"
                type="datetime-local"
                required
                value={end}
                onChange={(e) => {
                  setEnd(e.target.value)
                }}
              />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="mw-scope">{t('form.scope')}</FieldLabel>
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
                      {t(`scopeName.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {scope === 'all' ? null : (
              <Field>
                <FieldLabel htmlFor="mw-target">{t('form.target')}</FieldLabel>
                <Input
                  id="mw-target"
                  required
                  value={target}
                  placeholder={t(`form.targetPlaceholder.${scope}`)}
                  onChange={(e) => {
                    setTarget(e.target.value)
                  }}
                />
              </Field>
            )}
          </div>
          <Field>
            <FieldLabel htmlFor="mw-lead">{t('form.lead')}</FieldLabel>
            <Input
              id="mw-lead"
              inputMode="numeric"
              value={lead}
              onChange={(e) => {
                setLead(e.target.value)
              }}
            />
            <FieldDescription>{t('form.leadHint')}</FieldDescription>
          </Field>
          <Field orientation="horizontal">
            <Switch
              id="mw-freeze"
              checked={freeze}
              onCheckedChange={setFreeze}
            />
            <FieldLabel htmlFor="mw-freeze">{t('form.freeze')}</FieldLabel>
          </Field>
          {formError || create.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {formError ?? create.error?.message}
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setOpen(false)
                reset()
              }}
            >
              {t('form.cancel')}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {t('form.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { MaintenancePreview } from '../maintenance/MaintenancePreview'
import type {
  DomainMaintenance,
  SetDomainMaintenanceRequest,
} from '../../queries/domainMaintenance'

const MAINTENANCE_TITLE_MAX = 120
const MAINTENANCE_MESSAGE_MAX = 2000

interface Draft {
  title: string
  message: string
  until: string
  retryMinutes: string
  maxHours: string
  bypass: string
  healthBypass: boolean
}

// datetime-local wants local wall time without a zone.
function toLocalInput(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function draftFrom(m: DomainMaintenance | undefined): Draft {
  return {
    title: m?.title ?? '',
    message: m?.message ?? '',
    until: toLocalInput(m?.until),
    retryMinutes: m?.retry_after_seconds
      ? String(Math.round(m.retry_after_seconds / 60))
      : '',
    maxHours: '',
    bypass: (m?.bypass ?? []).join('\n'),
    healthBypass: m?.bypass_health_check ?? true,
  }
}

function toRequest(d: Draft): SetDomainMaintenanceRequest {
  const req: SetDomainMaintenanceRequest = {
    title: d.title.trim() || undefined,
    message: d.message.trim() || undefined,
    bypass: d.bypass
      .split(/[\s,]+/)
      .map((s) => s.trim())
      .filter(Boolean),
    bypass_health_check: d.healthBypass,
  }
  const minutes = Number(d.retryMinutes)
  if (d.retryMinutes.trim() && Number.isFinite(minutes) && minutes > 0) {
    req.retry_after_seconds = Math.round(minutes * 60)
  }
  const hours = Number(d.maxHours)
  if (d.maxHours.trim() && Number.isFinite(hours) && hours >= 0) {
    req.max_duration_seconds = Math.round(hours * 3600)
  }
  if (d.until) {
    const at = new Date(d.until)
    if (!Number.isNaN(at.getTime())) req.until = at.toISOString()
  }
  return req
}

// Page copy, end time and bypass rules for one domain's maintenance.
export function DomainMaintenanceForm({
  defaultMaxSeconds = 0,
  domain,
  current,
  enabled,
  pending,
  onSubmit,
}: {
  defaultMaxSeconds?: number
  domain: string
  current: DomainMaintenance | undefined
  enabled: boolean
  pending: boolean
  onSubmit: (req: SetDomainMaintenanceRequest) => void
}) {
  const { t } = useTranslation('domains')
  const [draft, setDraft] = useState<Draft>(() => draftFrom(current))
  const id = (field: string) => `maintenance-${field}-${domain}`
  const update = (patch: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...patch }))
  }

  return (
    <form
      className="mt-3 space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        onSubmit(toRequest(draft))
      }}
    >
      <Field>
        <FieldLabel htmlFor={id('title')}>
          {t('maintenance.form.title')}
        </FieldLabel>
        <Input
          id={id('title')}
          value={draft.title}
          maxLength={MAINTENANCE_TITLE_MAX}
          placeholder={t('maintenance.form.titlePlaceholder')}
          onChange={(e) => update({ title: e.target.value })}
          disabled={pending}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={id('message')}>
          {t('maintenance.form.message')}
        </FieldLabel>
        <Textarea
          id={id('message')}
          value={draft.message}
          maxLength={MAINTENANCE_MESSAGE_MAX}
          rows={3}
          placeholder={t('maintenance.form.messagePlaceholder')}
          onChange={(e) => update({ message: e.target.value })}
          disabled={pending}
        />
        <FieldDescription>{t('maintenance.form.plainText')}</FieldDescription>
      </Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={id('until')}>
            {t('maintenance.form.until')}
          </FieldLabel>
          <Input
            id={id('until')}
            type="datetime-local"
            value={draft.until}
            onChange={(e) => update({ until: e.target.value })}
            disabled={pending}
          />
          <FieldDescription>{t('maintenance.form.untilHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('retry')}>
            {t('maintenance.form.retryAfter')}
          </FieldLabel>
          <Input
            id={id('retry')}
            type="number"
            min={1}
            max={1440}
            inputMode="numeric"
            value={draft.retryMinutes}
            placeholder="5"
            onChange={(e) => update({ retryMinutes: e.target.value })}
            disabled={pending}
          />
          <FieldDescription>
            {t('maintenance.form.retryAfterHelp')}
          </FieldDescription>
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor={id('max')}>
          {t('maintenance.form.maxHours')}
        </FieldLabel>
        <Input
          id={id('max')}
          type="number"
          min={0}
          step="any"
          inputMode="decimal"
          value={draft.maxHours}
          placeholder={String(Math.round(defaultMaxSeconds / 360) / 10)}
          onChange={(e) => update({ maxHours: e.target.value })}
          disabled={pending || draft.until !== ''}
        />
        <FieldDescription>
          {draft.until !== ''
            ? t('maintenance.form.maxHoursUntil')
            : defaultMaxSeconds > 0
              ? t('maintenance.form.maxHoursDefault', {
                  hours: Math.round(defaultMaxSeconds / 360) / 10,
                })
              : t('maintenance.form.maxHoursNone')}
        </FieldDescription>
      </Field>
      <MaintenancePreview
        title={draft.title}
        message={draft.message}
        retryAfterSeconds={
          Math.round(Number(draft.retryMinutes || 0) * 60) || 0
        }
      />
      <Field>
        <FieldLabel htmlFor={id('bypass')}>
          {t('maintenance.form.bypass')}
        </FieldLabel>
        <Textarea
          id={id('bypass')}
          value={draft.bypass}
          rows={2}
          className="font-mono"
          placeholder={'203.0.113.7\n10.0.0.0/8'}
          onChange={(e) => update({ bypass: e.target.value })}
          disabled={pending}
        />
        <FieldDescription>{t('maintenance.form.bypassHelp')}</FieldDescription>
      </Field>
      <Field orientation="horizontal">
        <FieldLabel htmlFor={id('health')}>
          {t('maintenance.form.healthBypass')}
        </FieldLabel>
        <Switch
          id={id('health')}
          checked={draft.healthBypass}
          onCheckedChange={(checked) => update({ healthBypass: checked })}
          disabled={pending}
        />
      </Field>
      <Button type="submit" size="sm" disabled={pending}>
        {enabled ? t('maintenance.form.save') : t('maintenance.form.enable')}
      </Button>
    </form>
  )
}

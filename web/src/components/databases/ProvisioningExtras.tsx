import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { PendingSettings } from '../../queries/databaseProvision'

import type { ProvisionConnection } from '../../lib/provisionConnection'

// The API never returns passwords, so the URL stays masked and there is no reveal.
export function ReadyConnection({ c }: { c: ProvisionConnection }) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-1 rounded-md border border-border p-3">
      <dl className="grid grid-cols-2 gap-2 text-sm">
        <dt className="text-muted-foreground">{t('provision.host')}</dt>
        <dd className="font-mono break-all">{c.host}</dd>
        <dt className="text-muted-foreground">{t('provision.port')}</dt>
        <dd className="font-mono">{c.port}</dd>
        <dt className="text-muted-foreground">{t('provision.url')}</dt>
        <dd className="font-mono break-all">
          {`${c.scheme}://user:••••••••@${c.host}:${c.port}`}
        </dd>
      </dl>
      <p className="text-xs text-muted-foreground">
        {t('provision.passwordNote')}
      </p>
    </div>
  )
}

export function PendingSettingsForm({
  initial,
  busy,
  onSave,
  onCancel,
}: {
  initial: { version?: string }
  busy?: boolean
  onSave: (s: PendingSettings) => void
  onCancel: () => void
}) {
  const { t } = useTranslation('databases')
  const [version, setVersion] = useState(initial.version ?? '')
  const [memory, setMemory] = useState('')
  const [schedule, setSchedule] = useState('')

  function submit(e: FormEvent) {
    e.preventDefault()
    const mib = Number(memory)
    onSave({
      version: version.trim() || undefined,
      resources:
        mib > 0 ? { memory_bytes: Math.round(mib) * 1024 * 1024 } : undefined,
      backup_schedule: schedule.trim() || undefined,
    })
  }

  return (
    <form noValidate onSubmit={submit} className="space-y-2">
      <label className="block text-xs text-muted-foreground">
        {t('provision.editVersion')}
        <Input
          value={version}
          onChange={(e) => {
            setVersion(e.target.value)
          }}
        />
      </label>
      <label className="block text-xs text-muted-foreground">
        {t('provision.editMemory')}
        <Input
          inputMode="numeric"
          value={memory}
          onChange={(e) => {
            setMemory(e.target.value)
          }}
        />
      </label>
      <label className="block text-xs text-muted-foreground">
        {t('provision.editSchedule')}
        <Input
          value={schedule}
          onChange={(e) => {
            setSchedule(e.target.value)
          }}
        />
      </label>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={busy}>
          {t('provision.saveAndRetry')}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={onCancel}>
          {t('provision.cancel')}
        </Button>
      </div>
    </form>
  )
}

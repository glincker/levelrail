import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { useNotificationChannelsOptional } from '../../queries/notificationChannels'
import type {
  AutoUpgradeLevel,
  UpgradePolicy,
  UpgradePolicyInput,
} from '../../types/databaseUpgrades'
import {
  PRESET_CRON,
  draftToInput,
  policyToDraft,
  presetForCron,
  toggleChannel,
  type PolicyDraft,
  type WindowPreset,
} from './upgradeHelpers'

const AUTO_LEVELS: AutoUpgradeLevel[] = ['off', 'patch', 'minor']
const PRESETS: WindowPreset[] = ['daily', 'sundays', 'custom']

function isAutoLevel(v: unknown): v is AutoUpgradeLevel {
  return v === 'off' || v === 'patch' || v === 'minor'
}

function isPreset(v: unknown): v is WindowPreset {
  return v === 'daily' || v === 'sundays' || v === 'custom'
}

interface PolicyFormProps {
  policy: UpgradePolicy
  editable: boolean
  saving: boolean
  onSave: (input: UpgradePolicyInput) => void
  extraActions?: ReactNode
  idPrefix: string
}

/** PolicyForm edits one upgrade policy; the parent remounts it with a new key when the saved policy changes. */
export function PolicyForm({
  policy,
  editable,
  saving,
  onSave,
  extraActions,
  idPrefix,
}: PolicyFormProps) {
  const { t } = useTranslation('databaseUpgrades')
  const channels = useNotificationChannelsOptional()
  const [draft, setDraft] = useState<PolicyDraft>(() => policyToDraft(policy))
  const [preset, setPreset] = useState<WindowPreset>(() =>
    presetForCron(policy.window_cron),
  )

  function patch(next: Partial<PolicyDraft>) {
    setDraft((d) => ({ ...d, ...next }))
  }

  function pickPreset(next: WindowPreset) {
    setPreset(next)
    if (next !== 'custom') patch({ cron: PRESET_CRON[next] })
  }

  const channelList = channels.data ?? []

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (editable) onSave(draftToInput(draft))
      }}
    >
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-auto`}>
          {t('policy.autoUpgrade')}
        </FieldLabel>
        <Select
          value={draft.autoUpgrade}
          disabled={!editable}
          onValueChange={(v) => {
            if (isAutoLevel(v)) patch({ autoUpgrade: v })
          }}
        >
          <SelectTrigger id={`${idPrefix}-auto`} className="w-full sm:w-80">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {AUTO_LEVELS.map((level) => (
              <SelectItem key={level} value={level}>
                {t(`policy.auto.${level}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-preset`}>
            {t('policy.window')}
          </FieldLabel>
          <Select
            value={preset}
            disabled={!editable}
            onValueChange={(v) => {
              if (isPreset(v)) pickPreset(v)
            }}
          >
            <SelectTrigger id={`${idPrefix}-preset`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PRESETS.map((p) => (
                <SelectItem key={p} value={p}>
                  {t(`policy.preset.${p}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-cron`}>
            {t('policy.cron')}
          </FieldLabel>
          <Input
            id={`${idPrefix}-cron`}
            className="font-mono"
            value={draft.cron}
            disabled={!editable || preset !== 'custom'}
            onChange={(e) => {
              patch({ cron: e.target.value })
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-hours`}>
            {t('policy.duration')}
          </FieldLabel>
          <Input
            id={`${idPrefix}-hours`}
            type="number"
            min="0.25"
            step="0.25"
            value={draft.hours}
            disabled={!editable}
            onChange={(e) => {
              patch({ hours: e.target.value })
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-tz`}>
            {t('policy.timezone')}
          </FieldLabel>
          <Input
            id={`${idPrefix}-tz`}
            value={draft.timezone}
            disabled={!editable}
            onChange={(e) => {
              patch({ timezone: e.target.value })
            }}
          />
        </Field>
      </div>

      <div className="space-y-3">
        <div className="space-y-0.5">
          <div className="flex items-center gap-2 text-sm">
            <Switch
              id={`${idPrefix}-backup`}
              checked
              disabled
              aria-describedby={`${idPrefix}-backup-hint`}
            />
            <label htmlFor={`${idPrefix}-backup`}>
              {t('policy.backupBefore')}
            </label>
          </div>
          <FieldDescription id={`${idPrefix}-backup-hint`}>
            {t('policy.backupBeforeHint')}
          </FieldDescription>
        </div>
        <div className="space-y-0.5">
          <div className="flex items-center gap-2 text-sm">
            <Switch
              id={`${idPrefix}-verify`}
              checked={draft.verifyAfter}
              disabled={!editable}
              onCheckedChange={(v) => {
                patch({ verifyAfter: v })
              }}
            />
            <label htmlFor={`${idPrefix}-verify`}>
              {t('policy.verifyAfter')}
            </label>
          </div>
          <FieldDescription>{t('policy.verifyAfterHint')}</FieldDescription>
        </div>
        <div className="space-y-0.5">
          <div className="flex items-center gap-2 text-sm">
            <Switch
              id={`${idPrefix}-revert`}
              checked={draft.revertOnFailure}
              disabled={!editable}
              onCheckedChange={(v) => {
                patch({ revertOnFailure: v })
              }}
            />
            <label htmlFor={`${idPrefix}-revert`}>
              {t('policy.revertOnFailure')}
            </label>
          </div>
          <FieldDescription>{t('policy.revertOnFailureHint')}</FieldDescription>
        </div>
      </div>

      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t('policy.notify')}</legend>
        {channelList.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('policy.noChannels')}
          </p>
        ) : (
          channelList.map((c) => (
            <div key={c.id} className="flex items-center gap-2 text-sm">
              <Checkbox
                id={`${idPrefix}-ch-${c.id}`}
                checked={draft.notify.includes(c.id)}
                disabled={!editable}
                onCheckedChange={() => {
                  patch({ notify: toggleChannel(draft.notify, c.id) })
                }}
              />
              <label htmlFor={`${idPrefix}-ch-${c.id}`}>{c.name}</label>
            </div>
          ))
        )}
      </fieldset>

      <div className="flex flex-wrap items-center gap-2">
        {editable ? (
          <Button type="submit" size="sm" disabled={saving}>
            {saving ? t('policy.saving') : t('policy.save')}
          </Button>
        ) : null}
        {extraActions}
      </div>
    </form>
  )
}

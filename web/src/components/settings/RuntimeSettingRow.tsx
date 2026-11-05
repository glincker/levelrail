import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { DurationInput } from '@/components/ui/duration-input'
import { Input } from '@/components/ui/input'
import { SettingRow } from '@/components/ui/setting-row'
import { HighlightedText } from './HighlightedText'
import { RetentionDaysSelect } from './RetentionDaysSelect'
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
import {
  draftOf,
  needsConfirm,
  settingItemId,
  toWrite,
  valueLabelKey,
} from '../../lib/runtimeSettings'
import {
  ResetConfirmationError,
  useResetRuntimeSetting,
  useSetRuntimeSetting,
} from '../../queries/runtimeSettings'
import type {
  RuntimeSetting,
  RuntimeSettingValue,
  RuntimeSettingWrite,
} from '../../types/runtimeSettings'

const INT_RE = /^-?\d+$/

export function RuntimeSettingRow({
  setting,
  canEdit,
  showDefault = false,
  highlight = '',
}: {
  setting: RuntimeSetting
  canEdit: boolean
  /** Onboarding shows the safe default under the control. */
  showDefault?: boolean
  /** Search text to mark inside the title, description and key. */
  highlight?: string
}) {
  const { t } = useTranslation('runtimeSettings')
  const fieldId = useId()
  const save = useSetRuntimeSetting()
  const reset = useResetRuntimeSetting()
  const [draft, setDraft] = useState(() => draftOf(setting.value))
  const [syncedFrom, setSyncedFrom] = useState(setting.value)
  const [confirming, setConfirming] = useState<RuntimeSettingWrite | null>(null)
  const [confirmingReset, setConfirmingReset] = useState(false)
  const itemId = settingItemId(setting.key)
  // Locale item first, then the API text, so a new registry key still reads.
  const title = t(`items.${itemId}.title`, {
    defaultValue: setting.title || setting.key,
  })
  const description = t(`items.${itemId}.description`, {
    defaultValue: setting.description,
  })
  const busy = save.isPending || reset.isPending

  if (syncedFrom !== setting.value) {
    setSyncedFrom(setting.value)
    setDraft(draftOf(setting.value))
  }

  const valueText = (v: RuntimeSettingValue) => {
    const key = valueLabelKey(v)
    if (key) return t(key)
    return Array.isArray(v) ? v.join(', ') : String(v)
  }

  function submit(next: RuntimeSettingWrite, confirmWeakening = false) {
    save.mutate(
      { key: setting.key, value: next, confirmWeakening },
      {
        onSuccess: () => {
          setConfirming(null)
          toast.add({ type: 'success', title: t('toast.saved', { title }) })
        },
        onError: (err) => {
          if (confirmWeakening) return
          toast.add({
            type: 'error',
            title: t('toast.saveError', { title, message: err.message }),
          })
        },
      },
    )
  }

  function doReset(confirmWeakening: boolean) {
    reset.mutate(
      { key: setting.key, confirmWeakening },
      {
        onSuccess: () => {
          setConfirmingReset(false)
          toast.add({ type: 'success', title: t('toast.reset', { title }) })
        },
        onError: (err) => {
          // The server asks first when the reset restores a weaker value.
          if (!confirmWeakening && err instanceof ResetConfirmationError) {
            setConfirmingReset(true)
            return
          }
          if (confirmWeakening) return
          toast.add({
            type: 'error',
            title: t('toast.resetError', { title, message: err.message }),
          })
        },
      },
    )
  }

  function request(next: RuntimeSettingWrite) {
    if (needsConfirm(setting, next)) {
      setConfirming(next)
      return
    }
    submit(next)
  }

  const intInvalid = setting.kind === 'int' && !INT_RE.test(draft.trim())
  const unchanged = draft === draftOf(setting.value)

  const badges = (
    <>
      {setting.risk !== 'normal' ? (
        setting.weaker_than_default ? (
          <Badge variant="destructive">{t('badges.weakens')}</Badge>
        ) : (
          <Badge variant="muted">{t('badges.security')}</Badge>
        )
      ) : null}
      {setting.source !== 'default' ? (
        <Badge
          title={t('row.defaultHint', { value: valueText(setting.default) })}
          variant={setting.source === 'saved' ? 'info' : 'accent'}
        >
          {t(`source.${setting.source}`)}
        </Badge>
      ) : null}
      {setting.apply === 'restart' || setting.pending_restart ? (
        <Badge variant={setting.pending_restart ? 'warning' : 'muted'}>
          {t('badges.restart')}
        </Badge>
      ) : null}
    </>
  )

  return (
    <SettingRow
      title={<HighlightedText text={title} query={highlight} />}
      description={<HighlightedText text={description} query={highlight} />}
      badge={badges}
    >
      <div className="space-y-1" data-setting={setting.key}>
        {setting.kind === 'bool' ? (
          <div className="flex items-center gap-3">
            <Switch
              id={fieldId}
              checked={setting.value === true}
              disabled={!canEdit || busy}
              aria-label={title}
              onCheckedChange={(next) => {
                request(next)
              }}
            />
            <span className="text-sm text-muted-foreground">
              {setting.value === true ? t('row.on') : t('row.off')}
            </span>
          </div>
        ) : (
          <div className="flex flex-wrap items-start gap-2">
            {setting.kind === 'duration' ? (
              <DurationInput
                id={fieldId}
                value={draft}
                disabled={!canEdit || busy}
                onChange={setDraft}
              />
            ) : setting.kind === 'int' && setting.allowed_values?.length ? (
              <RetentionDaysSelect
                id={fieldId}
                value={Number(draft)}
                allowed={setting.allowed_values}
                unit={setting.key.endsWith('_hours') ? 'hours' : 'days'}
                disabled={!canEdit || busy}
                ariaLabel={title}
                onChange={(days) => {
                  setDraft(String(days))
                }}
              />
            ) : setting.kind === 'string' && setting.allowed_strings?.length ? (
              <Select
                value={draft}
                disabled={!canEdit || busy}
                onValueChange={(next) => {
                  if (next !== null) setDraft(next)
                }}
              >
                <SelectTrigger id={fieldId} size="sm" aria-label={title}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {setting.allowed_strings.map((word) => (
                    <SelectItem key={word} value={word}>
                      {t(`options.${word}`, { defaultValue: word })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : setting.kind === 'int' ? (
              <Input
                id={fieldId}
                type="number"
                inputMode="numeric"
                value={draft}
                disabled={!canEdit || busy}
                aria-label={title}
                aria-invalid={intInvalid}
                onChange={(e) => {
                  setDraft(e.target.value)
                }}
              />
            ) : (
              <Textarea
                id={fieldId}
                rows={3}
                value={draft}
                disabled={!canEdit || busy}
                aria-label={t('row.listLabel', { title })}
                placeholder={t('row.listPlaceholder')}
                onChange={(e) => {
                  setDraft(e.target.value)
                }}
              />
            )}
            {canEdit ? (
              <Button
                type="button"
                size="sm"
                disabled={busy || unchanged || intInvalid}
                onClick={() => {
                  request(toWrite(setting, draft))
                }}
              >
                {save.isPending ? t('row.saving') : t('row.save')}
              </Button>
            ) : null}
            {intInvalid ? (
              <p className="w-full text-xs text-destructive">
                {t('row.invalid')}
              </p>
            ) : null}
          </div>
        )}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {showDefault || setting.source !== 'default' ? (
            <span>
              {showDefault
                ? t('onboarding.safeDefault', {
                    value: valueText(setting.default),
                  })
                : t('row.defaultHint', { value: valueText(setting.default) })}
            </span>
          ) : null}
          <code className="font-mono text-[11px] text-muted-foreground/70">
            <HighlightedText text={setting.key} query={highlight} />
          </code>
          {setting.env ? (
            <button
              type="button"
              className="inline-flex min-h-7 items-center gap-1.5 rounded px-1.5 font-mono text-[11px] text-muted-foreground/70 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              title={t('row.envCopy', { env: setting.env })}
              aria-label={t('row.envCopy', { env: setting.env })}
              onClick={() => {
                void navigator.clipboard.writeText(setting.env)
              }}
            >
              {setting.env}
              <CopyGlassIcon size="1.6em" aria-hidden="true" />
            </button>
          ) : null}
          {setting.source === 'saved' && setting.updated_by ? (
            <span>{t('row.updatedBy', { actor: setting.updated_by })}</span>
          ) : null}
          {canEdit && setting.source === 'saved' ? (
            <Button
              type="button"
              size="xs"
              variant="ghost"
              disabled={busy}
              onClick={() => {
                doReset(false)
              }}
            >
              <RotateCcwGlassIcon size="1.4em" aria-hidden="true" />
              {reset.isPending ? t('row.resetting') : t('row.reset')}
            </Button>
          ) : null}
        </div>
      </div>
      <ConfirmDialog
        open={confirming !== null || confirmingReset}
        onOpenChange={(open) => {
          if (!open) {
            setConfirming(null)
            setConfirmingReset(false)
          }
        }}
        title={t('confirm.title')}
        description={t('confirm.body', { title })}
        consequences={[t('confirm.consequence')]}
        confirmLabel={t('confirm.action')}
        cancelLabel={t('confirm.cancel')}
        tone="destructive"
        pending={confirmingReset ? reset.isPending : save.isPending}
        error={
          (confirmingReset ? reset.error?.message : save.error?.message) ?? null
        }
        onConfirm={() => {
          if (confirmingReset) doReset(true)
          else if (confirming !== null) submit(confirming, true)
        }}
      />
    </SettingRow>
  )
}

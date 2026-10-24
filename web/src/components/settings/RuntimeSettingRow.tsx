import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { DurationInput } from '@/components/ui/duration-input'
import { Input } from '@/components/ui/input'
import { SettingRow } from '@/components/ui/setting-row'
import { RetentionDaysSelect } from './RetentionDaysSelect'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  draftOf,
  needsConfirm,
  toWrite,
  valueLabelKey,
} from '../../lib/runtimeSettings'
import {
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
}: {
  setting: RuntimeSetting
  canEdit: boolean
  /** Onboarding shows the safe default under the control. */
  showDefault?: boolean
}) {
  const { t } = useTranslation('runtimeSettings')
  const fieldId = useId()
  const save = useSetRuntimeSetting()
  const reset = useResetRuntimeSetting()
  const [draft, setDraft] = useState(() => draftOf(setting.value))
  const [syncedFrom, setSyncedFrom] = useState(setting.value)
  const [confirming, setConfirming] = useState<RuntimeSettingWrite | null>(null)
  const title = setting.title || setting.key
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
      <Badge
        variant={
          setting.source === 'saved'
            ? 'info'
            : setting.source === 'env'
              ? 'accent'
              : 'muted'
        }
      >
        {t(`source.${setting.source}`)}
      </Badge>
      {setting.pending_restart ? (
        <Badge variant="warning">{t('badges.restart')}</Badge>
      ) : null}
    </>
  )

  return (
    <SettingRow title={title} description={setting.description} badge={badges}>
      <div className="space-y-2" data-setting={setting.key}>
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
          <div className="space-y-2">
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
                disabled={!canEdit || busy}
                ariaLabel={title}
                onChange={(days) => {
                  setDraft(String(days))
                }}
              />
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
            {intInvalid ? (
              <p className="text-xs text-destructive">{t('row.invalid')}</p>
            ) : null}
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
          {setting.env ? (
            <span>{t('row.envHint', { env: setting.env })}</span>
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
                reset.mutate(setting.key, {
                  onSuccess: () => {
                    toast.add({
                      type: 'success',
                      title: t('toast.reset', { title }),
                    })
                  },
                  onError: (err) => {
                    toast.add({
                      type: 'error',
                      title: t('toast.resetError', {
                        title,
                        message: err.message,
                      }),
                    })
                  },
                })
              }}
            >
              <ArrowCounterClockwiseIcon aria-hidden="true" />
              {reset.isPending ? t('row.resetting') : t('row.reset')}
            </Button>
          ) : null}
        </div>
      </div>
      <ConfirmDialog
        open={confirming !== null}
        onOpenChange={(open) => {
          if (!open) setConfirming(null)
        }}
        title={t('confirm.title')}
        description={t('confirm.body', { title })}
        consequences={[t('confirm.consequence')]}
        confirmLabel={t('confirm.action')}
        cancelLabel={t('confirm.cancel')}
        tone="destructive"
        pending={save.isPending}
        error={save.error?.message ?? null}
        onConfirm={() => {
          if (confirming !== null) submit(confirming, true)
        }}
      />
    </SettingRow>
  )
}

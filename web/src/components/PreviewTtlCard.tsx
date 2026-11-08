import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SaveStatus, type SaveState } from '@/components/ui/save-status'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../queries/runtimeSettings'

export const PREVIEW_TTL_SETTING = 'deploy.preview_ttl_hours'

function envInt(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isInteger(n) && n >= 0 ? n : fallback
}

// Mirrors the registry range; env lets a deployment widen it without a rebuild.
const TTL_MAX = envInt(import.meta.env.VITE_PREVIEW_TTL_MAX_HOURS, 8760)

// PreviewTtlCard edits the platform-wide preview lifetime. It renders nothing
// when the setting cannot be read (403 or an older server).
export function PreviewTtlCard() {
  const { t } = useTranslation('previews')
  const { data } = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const [draft, setDraft] = useState<string | null>(null)
  const [state, setState] = useState<SaveState>('idle')
  const setting = data?.settings.find((s) => s.key === PREVIEW_TTL_SETTING)
  if (!setting) return null

  const text = draft ?? String(setting.value)
  const n = Number(text)
  const valid =
    text.trim() !== '' && Number.isInteger(n) && n >= 0 && n <= TTL_MAX
  const dirty = draft !== null && text !== String(setting.value)

  return (
    <form
      className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border px-4 py-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (!valid || !dirty) return
        setState('saving')
        save.mutate(
          { key: setting.key, value: String(n) },
          {
            onSuccess: () => {
              setDraft(null)
              setState('saved')
            },
            onError: () => {
              setState('error')
            },
          },
        )
      }}
    >
      <label htmlFor="preview-ttl-hours" className="text-sm font-medium">
        {t('ttl.label')}
      </label>
      <Input
        id="preview-ttl-hours"
        type="number"
        inputMode="numeric"
        min={0}
        max={TTL_MAX}
        className="w-24"
        value={text}
        aria-invalid={!valid}
        aria-describedby="preview-ttl-hours-note"
        onChange={(e) => {
          setDraft(e.target.value)
          setState('idle')
        }}
      />
      <Button
        type="submit"
        size="sm"
        disabled={!valid || !dirty || save.isPending}
      >
        {t('ttl.save')}
      </Button>
      <SaveStatus state={state} />
      <p
        id="preview-ttl-hours-note"
        className="basis-full text-xs text-muted-foreground"
      >
        {valid ? t('ttl.note') : t('ttl.invalid', { max: TTL_MAX })}
      </p>
    </form>
  )
}

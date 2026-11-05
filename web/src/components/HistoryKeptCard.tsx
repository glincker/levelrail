import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SaveStatus, type SaveState } from '@/components/ui/save-status'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../queries/runtimeSettings'

export const HISTORY_KEEP_SETTING = 'scheduled_tasks.history_keep'

function envInt(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : fallback
}

// Range mirrors the registry; env lets a deployment widen it without a rebuild.
const KEEP_MIN = envInt(import.meta.env.VITE_JOBS_HISTORY_KEEP_MIN, 1)
const KEEP_MAX = envInt(import.meta.env.VITE_JOBS_HISTORY_KEEP_MAX, 10000)

// HistoryKeptCard edits how many past runs each job keeps. It renders nothing
// when the setting cannot be read (403 or an older server).
export function HistoryKeptCard() {
  const { t } = useTranslation('jobs')
  const { data } = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const [draft, setDraft] = useState<string | null>(null)
  const [state, setState] = useState<SaveState>('idle')
  const setting = data?.settings.find((s) => s.key === HISTORY_KEEP_SETTING)
  if (!setting) return null

  const text = draft ?? String(setting.value)
  const n = Number(text)
  const valid = Number.isInteger(n) && n >= KEEP_MIN && n <= KEEP_MAX
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
      <label htmlFor="jobs-history-keep" className="text-sm font-medium">
        {t('historyKept.label')}
      </label>
      <Input
        id="jobs-history-keep"
        type="number"
        inputMode="numeric"
        min={KEEP_MIN}
        max={KEEP_MAX}
        className="w-24"
        value={text}
        aria-invalid={!valid}
        aria-describedby="jobs-history-keep-note"
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
        {t('historyKept.save')}
      </Button>
      <SaveStatus state={state} />
      <p
        id="jobs-history-keep-note"
        className="basis-full text-xs text-muted-foreground"
      >
        {valid
          ? t('historyKept.note')
          : t('historyKept.invalid', { min: KEEP_MIN, max: KEEP_MAX })}
      </p>
    </form>
  )
}

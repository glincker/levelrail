import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  CloudCheckGlassIcon,
  DownloadGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  exportAppearanceJSON,
  parseImport,
  syncLineKey,
  type SyncLineKey,
} from '@/lib/appearanceSync'
import {
  appearanceSync,
  useAppearanceSyncState,
} from '@/queries/appearanceSync'

const TICK_MS = 30_000

const LINES: Record<SyncLineKey, (t: TFunction<'settings'>) => string> = {
  off: (t) => t('appearancePage.sync.off'),
  syncing: (t) => t('appearancePage.sync.syncing'),
  tooLarge: (t) => t('appearancePage.sync.tooLarge'),
  notSynced: (t) => t('appearancePage.sync.notSynced'),
  synced: (t) => t('appearancePage.sync.notSynced'),
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    const id = setInterval(() => setNow(Date.now()), TICK_MS)
    return () => clearInterval(id)
  }, [active])
  return now
}

function download(text: string) {
  const url = URL.createObjectURL(
    new Blob([text], { type: 'application/json' }),
  )
  const a = document.createElement('a')
  a.href = url
  a.download = 'levelrail-appearance.json'
  a.click()
  URL.revokeObjectURL(url)
}

export function AppearanceSyncRow() {
  const { t, i18n } = useTranslation('settings')
  const state = useAppearanceSyncState()
  const fileRef = useRef<HTMLInputElement>(null)
  const [importMsg, setImportMsg] = useState<string | null>(null)
  const now = useNow(state.status === 'synced')

  if (state.available !== 'yes') return null

  const key = syncLineKey(state)
  let line: string = LINES[key](t)
  if (key === 'synced' && state.syncedAt !== null) {
    const minutes = Math.round((state.syncedAt - now) / 60_000)
    const when =
      minutes === 0
        ? t('appearancePage.sync.justNow')
        : new Intl.RelativeTimeFormat(i18n.language, {
            numeric: 'auto',
          }).format(minutes, 'minute')
    line = t('appearancePage.sync.synced', { when })
  }

  const onFile = async (file: File | undefined) => {
    if (!file) return
    const parsed = parseImport(await file.text())
    if (!parsed) {
      setImportMsg(t('appearancePage.sync.importBad'))
      return
    }
    appearanceSync.importValue(parsed)
    setImportMsg(t('appearancePage.sync.imported'))
  }

  const labelId = 'appearance-sync-label'
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border bg-card px-3 py-2">
      <CloudCheckGlassIcon size="1.4em" aria-hidden="true" />
      <div className="flex min-w-0 flex-1 flex-col">
        <span id={labelId} className="text-sm font-medium">
          {t('appearancePage.sync.label')}
        </span>
        <span
          role="status"
          aria-live="polite"
          className="truncate text-xs text-muted-foreground"
        >
          {importMsg ?? line}
        </span>
      </div>
      <Switch
        aria-labelledby={labelId}
        checked={state.enabled}
        onCheckedChange={(v) => appearanceSync.setEnabled(v)}
      />
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => download(exportAppearanceJSON())}
        >
          <DownloadGlassIcon size="1.4em" aria-hidden="true" />
          {t('appearancePage.sync.export')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => fileRef.current?.click()}
        >
          <UploadGlassIcon size="1.4em" aria-hidden="true" />
          {t('appearancePage.sync.import')}
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          aria-label={t('appearancePage.sync.importLabel')}
          className="sr-only"
          onChange={(e) => {
            void onFile(e.target.files?.[0])
            e.target.value = ''
          }}
        />
      </div>
    </div>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { DEFAULT_UNLOCK_MINUTES } from '../../hooks/useWriteUnlock'

const BASE_MINUTES = [5, 15, 30]

/** Unlock writes dialog: a typed database name and a window length gate the unlock. */
export function WriteUnlockDialog({
  databaseName,
  open,
  onOpenChange,
  onUnlock,
}: Readonly<{
  databaseName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onUnlock: (minutes: number) => void
}>) {
  const { t } = useTranslation('dbExplorerStrip')
  const [typed, setTyped] = useState('')
  const [minutes, setMinutes] = useState(
    BASE_MINUTES.includes(DEFAULT_UNLOCK_MINUTES) ? DEFAULT_UNLOCK_MINUTES : 15,
  )

  function close(next: boolean) {
    if (!next) setTyped('')
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <TriangleAlertGlassIcon
              className="size-6 text-tone-warning"
              aria-hidden="true"
            />
            {t('unlock.title')}
          </DialogTitle>
          <DialogDescription>
            {t('unlock.description', { name: databaseName })}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-end gap-3">
          <label className="min-w-40 flex-1 space-y-1 text-sm">
            <span>{t('unlock.confirmLabel', { name: databaseName })}</span>
            <Input
              value={typed}
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => {
                setTyped(e.target.value)
              }}
            />
          </label>
          <label className="space-y-1 text-sm">
            <span className="block">{t('unlock.duration')}</span>
            <NativeSelect
              value={minutes}
              onChange={(e) => {
                setMinutes(Number(e.target.value))
              }}
            >
              {[...new Set([...BASE_MINUTES, minutes])]
                .sort((a, b) => a - b)
                .map((m) => (
                  <option key={m} value={m}>
                    {t('unlock.minutes', { count: m })}
                  </option>
                ))}
            </NativeSelect>
          </label>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              close(false)
            }}
          >
            {t('unlock.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={typed !== databaseName}
            onClick={() => {
              onUnlock(minutes)
              close(false)
            }}
          >
            {t('unlock.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

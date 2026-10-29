import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
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
import { toast } from '@/components/ui/toast'
import { formatDate } from '../lib/format'
import { isoToLocalInputValue, localInputValueToISO } from '../lib/pitrTime'
import { useTriggerPITRRestore } from '../queries/pitr'
import type { ApiError } from '../lib/apiError'
import type { BaseBackupHistoryRecord } from '../types/pitr'

interface PITRRestoreIntoNewDialogProps {
  databaseName: string
  engine: string
  pitrWindow:
    | { has_base_backup: boolean; window_start?: string; window_end?: string }
    | undefined
  succeededBackups: BaseBackupHistoryRecord[]
}

// MySQL and MariaDB restores never overwrite the source: they create a new
// database, so there is no destructive confirmation to type.
export function PITRRestoreIntoNewDialog({
  databaseName,
  engine,
  pitrWindow,
  succeededBackups,
}: PITRRestoreIntoNewDialogProps) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const [targetLocal, setTargetLocal] = useState('')
  const [newName, setNewName] = useState(`${databaseName}-restore`)
  const triggerRestore = useTriggerPITRRestore(databaseName)

  const canRestore = Boolean(
    pitrWindow?.has_base_backup &&
    pitrWindow.window_start &&
    pitrWindow.window_end,
  )
  const targetISO = targetLocal ? localInputValueToISO(targetLocal) : null

  const chosenBackup = useMemo(() => {
    if (!targetISO) return null
    const targetMs = new Date(targetISO).getTime()
    return (
      succeededBackups
        .filter((b) => new Date(b.started_at).getTime() <= targetMs)
        .sort(
          (a, b) =>
            new Date(b.started_at).getTime() - new Date(a.started_at).getTime(),
        )[0] ?? null
    )
  }, [succeededBackups, targetISO])

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setTargetLocal('')
      triggerRestore.reset()
    }
  }

  const nameValid = newName.trim() !== '' && newName.trim() !== databaseName

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button variant="outline" size="sm" disabled={!canRestore} />}
      >
        <RestoreGlassIcon className="size-5" aria-hidden="true" />
        {t('pitrNew.restoreButton')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>
            {t('pitrNew.title', { name: databaseName })}
          </DialogTitle>
          <DialogDescription>
            {t('pitrNew.description', { name: databaseName })}
          </DialogDescription>
        </DialogHeader>

        <Field>
          <FieldLabel htmlFor="pitr-new-target-time">
            {t('pitrNew.restoreTo')}
          </FieldLabel>
          <Input
            id="pitr-new-target-time"
            type="datetime-local"
            step={1}
            min={
              pitrWindow?.window_start
                ? isoToLocalInputValue(pitrWindow.window_start)
                : undefined
            }
            max={
              pitrWindow?.window_end
                ? isoToLocalInputValue(pitrWindow.window_end)
                : undefined
            }
            value={targetLocal}
            onChange={(event) => {
              setTargetLocal(event.target.value)
            }}
          />
          <FieldDescription>
            {pitrWindow?.window_start && pitrWindow.window_end
              ? t('pitrNew.window', {
                  start: formatDate(pitrWindow.window_start, '-'),
                  end: formatDate(pitrWindow.window_end, '-'),
                })
              : null}
          </FieldDescription>
        </Field>

        {targetISO && chosenBackup ? (
          <p className="text-xs text-muted-foreground">
            {t('pitrNew.replayFrom', {
              when: formatDate(chosenBackup.started_at, '-'),
            })}
          </p>
        ) : null}
        {targetISO && !chosenBackup ? (
          <p className="text-xs text-destructive">{t('pitrNew.noBackup')}</p>
        ) : null}

        <Field>
          <FieldLabel htmlFor="pitr-new-name">
            {t('pitrNew.newName')}
          </FieldLabel>
          <Input
            id="pitr-new-name"
            autoComplete="off"
            spellCheck={false}
            value={newName}
            onChange={(event) => {
              setNewName(event.target.value)
            }}
          />
          <FieldDescription>
            {t('pitrNew.newNameHint', { engine })}
          </FieldDescription>
        </Field>

        {triggerRestore.isError ? (
          <p className="text-sm text-destructive">
            {triggerRestore.error.message}
          </p>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('pitrNew.cancel')}
          </Button>
          <Button
            type="button"
            disabled={
              !nameValid ||
              !targetISO ||
              !chosenBackup ||
              triggerRestore.isPending
            }
            onClick={() => {
              if (!targetISO || !chosenBackup) return
              triggerRestore.mutate(
                {
                  base_backup_id: chosenBackup.id,
                  target_time: targetISO,
                  restore_as: newName.trim(),
                },
                {
                  onSuccess: () => {
                    setOpen(false)
                    setTargetLocal('')
                    toast.add({
                      title: t('pitrNew.startedTitle'),
                      description: t('pitrNew.startedBody'),
                      type: 'success',
                    })
                  },
                  onError: (error: ApiError) => {
                    toast.add({
                      title: t('pitrNew.failedTitle'),
                      description: error.message,
                      type: 'error',
                    })
                  },
                },
              )
            }}
          >
            {triggerRestore.isPending
              ? t('pitrNew.submitting')
              : t('pitrNew.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

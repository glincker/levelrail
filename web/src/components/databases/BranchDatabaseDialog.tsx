import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { useBackupHistory } from '../../queries/backupHistory'
import { useCreateDatabaseBranch } from '../../queries/databaseBranch'

// Point-in-time branches exist only where the restore lands in a new database; the server checks PITR is on.
const PITR_BRANCH_ENGINES = ['mysql', 'mariadb']

export function BranchDatabaseDialog({
  name,
  engine,
  open,
  onOpenChange,
}: {
  name: string
  engine: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('databases')
  const history = useBackupHistory(name)
  const branch = useCreateDatabaseBranch(name)
  const [newName, setNewName] = useState('')
  const [mode, setMode] = useState<'backup' | 'time'>('backup')
  const [backupId, setBackupId] = useState('')
  const [at, setAt] = useState('')

  const backups = (history.data ?? []).filter((b) => b.status === 'succeeded')
  const canUseTime = PITR_BRANCH_ENGINES.includes(engine)
  const trimmed = newName.trim()
  const source = mode === 'backup' ? backupId || backups[0]?.id : at
  const canSubmit = trimmed.length > 0 && trimmed !== name && Boolean(source)

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) {
      setNewName('')
      setAt('')
      branch.reset()
    }
  }

  function submit() {
    branch.mutate(
      mode === 'backup'
        ? { name: trimmed, backup_id: source }
        : { name: trimmed, at: new Date(at).toISOString() },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: t('branch.started', { name: created.name }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('branch.title')}</DialogTitle>
          <DialogDescription>
            {t('branch.description', { name })}
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="branch-name">{t('branch.name')}</FieldLabel>
          <Input
            id="branch-name"
            autoComplete="off"
            spellCheck={false}
            placeholder={`${name}-branch`}
            value={newName}
            onChange={(event) => {
              setNewName(event.target.value)
            }}
          />
        </Field>
        {canUseTime ? (
          <Field>
            <FieldLabel htmlFor="branch-mode">{t('branch.from')}</FieldLabel>
            <NativeSelect
              id="branch-mode"
              value={mode}
              onChange={(event) => {
                setMode(event.target.value === 'time' ? 'time' : 'backup')
              }}
            >
              <option value="backup">{t('branch.fromBackup')}</option>
              <option value="time">{t('branch.fromTime')}</option>
            </NativeSelect>
          </Field>
        ) : null}
        {mode === 'backup' ? (
          <Field>
            <FieldLabel htmlFor="branch-backup">
              {t('branch.backup')}
            </FieldLabel>
            <NativeSelect
              id="branch-backup"
              value={source ?? ''}
              disabled={backups.length === 0}
              onChange={(event) => {
                setBackupId(event.target.value)
              }}
            >
              {backups.length === 0 ? (
                <option value="">{t('branch.noBackups')}</option>
              ) : null}
              {backups.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.finished_at ?? b.started_at}
                </option>
              ))}
            </NativeSelect>
          </Field>
        ) : (
          <Field>
            <FieldLabel htmlFor="branch-at">{t('branch.time')}</FieldLabel>
            <Input
              id="branch-at"
              type="datetime-local"
              value={at}
              onChange={(event) => {
                setAt(event.target.value)
              }}
            />
          </Field>
        )}
        {branch.isError ? (
          <p className="text-sm text-destructive">{branch.error.message}</p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {t('branch.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!canSubmit || branch.isPending}
            onClick={submit}
          >
            {branch.isPending ? t('branch.starting') : t('branch.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  DownloadGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { useAbilities } from '@/hooks/useAbilities'
import {
  backupDownloadURL,
  useBackupHistory,
} from '../../queries/backupHistory'
import { useTriggerCloneRestore } from '../../queries/cloneRestore'
import {
  uploadDatabaseImport,
  useImportCheck,
} from '../../queries/databasesData'

export function DatabaseDataActionsCard({ database }: { database: string }) {
  const { t } = useTranslation('databases')
  const { can } = useAbilities()
  const history = useBackupHistory(database)
  const latest = (history.data ?? []).find((b) => b.status === 'succeeded')

  const canDownload = can('read:sensitive')
  const canClone = can('write:sensitive')
  const canImport = can('root')

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('dataActions.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {t('dataActions.description')}
        </p>
        <section aria-label={t('dataActions.download.title')}>
          <div className="flex flex-wrap items-center gap-2">
            {canDownload && latest ? (
              <Button
                size="sm"
                variant="outline"
                nativeButton={false}
                render={
                  <a href={backupDownloadURL(database, latest.id)} download />
                }
              >
                <DownloadGlassIcon className="size-4" aria-hidden="true" />
                {t('dataActions.download.action')}
              </Button>
            ) : (
              <Button size="sm" variant="outline" disabled>
                <DownloadGlassIcon className="size-4" aria-hidden="true" />
                {t('dataActions.download.action')}
              </Button>
            )}
            <span className="text-xs text-muted-foreground">
              {!canDownload
                ? t('dataActions.needsReadSensitive')
                : latest
                  ? t('dataActions.download.source', {
                      when: new Date(latest.started_at).toLocaleString(),
                    })
                  : t('dataActions.download.none')}
            </span>
          </div>
        </section>
        <CloneForm
          database={database}
          backupId={latest?.id}
          allowed={canClone}
        />
        <ImportForm database={database} allowed={canImport} />
      </CardContent>
    </Card>
  )
}

function CloneForm({
  database,
  backupId,
  allowed,
}: {
  database: string
  backupId: string | undefined
  allowed: boolean
}) {
  const { t } = useTranslation('databases')
  const [name, setName] = useState(`${database}-clone`)
  const clone = useTriggerCloneRestore(database)
  const ready = allowed && !!backupId && name.trim() !== ''

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!ready || !backupId) return
    clone.mutate(
      { backup_id: backupId, new_name: name.trim() },
      {
        onSuccess: () =>
          toast.add({
            title: t('dataActions.clone.started', { name: name.trim() }),
            type: 'success',
          }),
        onError: (error) =>
          toast.add({
            title: t('dataActions.clone.failed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <form
      onSubmit={submit}
      className="flex flex-wrap items-end gap-2"
      aria-label={t('dataActions.clone.title')}
    >
      <div className="min-w-48 flex-1 space-y-1">
        <Label htmlFor={`clone-name-${database}`}>
          {t('dataActions.clone.label')}
        </Label>
        <Input
          id={`clone-name-${database}`}
          value={name}
          disabled={!allowed}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <Button type="submit" size="sm" disabled={!ready || clone.isPending}>
        <CopyGlassIcon className="size-4" aria-hidden="true" />
        {clone.isPending
          ? t('dataActions.clone.cloning')
          : t('dataActions.clone.action')}
      </Button>
      <p className="w-full text-xs text-muted-foreground">
        {!allowed
          ? t('dataActions.needsWriteSensitive')
          : backupId
            ? t('dataActions.clone.hint')
            : t('dataActions.clone.none')}
      </p>
    </form>
  )
}

function ImportForm({
  database,
  allowed,
}: {
  database: string
  allowed: boolean
}) {
  const { t } = useTranslation('databases')
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [confirm, setConfirm] = useState('')
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const check = useImportCheck(database, allowed && file !== null)

  const nonEmpty = check.data ? !check.data.empty : false
  const confirmed = !nonEmpty || confirm === database
  const uploading = progress !== null
  const ready = allowed && !!file && !!check.data && confirmed && !uploading

  async function runImport(e: React.FormEvent) {
    e.preventDefault()
    if (!ready || !file) return
    setError(null)
    setProgress(0)
    const { promise } = uploadDatabaseImport(
      database,
      file,
      { force: nonEmpty, confirm },
      setProgress,
    )
    try {
      await promise
      toast.add({ title: t('dataActions.import.done'), type: 'success' })
      setFile(null)
      setConfirm('')
      if (fileRef.current) fileRef.current.value = ''
    } catch (err) {
      setError(
        err instanceof Error ? err.message : t('dataActions.import.failed'),
      )
    } finally {
      setProgress(null)
    }
  }

  return (
    <form
      onSubmit={(e) => void runImport(e)}
      className="space-y-2"
      aria-label={t('dataActions.import.title')}
    >
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-48 flex-1 space-y-1">
          <Label htmlFor={`import-file-${database}`}>
            {t('dataActions.import.label')}
          </Label>
          <Input
            id={`import-file-${database}`}
            ref={fileRef}
            type="file"
            disabled={!allowed || uploading}
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null)
              setConfirm('')
              setError(null)
            }}
          />
        </div>
        {nonEmpty ? (
          <div className="min-w-40 space-y-1">
            <Label htmlFor={`import-confirm-${database}`}>
              {t('dataActions.import.confirmLabel', { name: database })}
            </Label>
            <Input
              id={`import-confirm-${database}`}
              value={confirm}
              disabled={uploading}
              autoComplete="off"
              onChange={(e) => setConfirm(e.target.value)}
            />
          </div>
        ) : null}
        <Button
          type="submit"
          size="sm"
          variant={nonEmpty ? 'destructive' : 'default'}
          disabled={!ready}
        >
          <UploadGlassIcon className="size-4" aria-hidden="true" />
          {uploading
            ? t('dataActions.import.uploading')
            : nonEmpty
              ? t('dataActions.import.replace')
              : t('dataActions.import.action')}
        </Button>
      </div>
      {!allowed ? (
        <p className="text-xs text-muted-foreground">
          {t('dataActions.needsRoot')}
        </p>
      ) : null}
      {nonEmpty ? (
        <Alert variant="warning">
          <AlertDescription>
            {t('dataActions.import.nonEmpty')}
          </AlertDescription>
        </Alert>
      ) : null}
      {check.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{check.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {uploading ? (
        <Progress
          value={Math.round(progress * 100)}
          aria-label={t('dataActions.import.progress')}
        />
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}

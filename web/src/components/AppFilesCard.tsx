import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FileTextGlassIcon,
  PencilGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useAppFiles,
  useDeleteAppFile,
  usePutAppFile,
  type AppFile,
} from '../queries/appFiles'

const MODE_PATTERN = /^0?[0-7]{3,4}$/
const DEFAULT_MODE = '0644'

export function AppFilesCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appFiles')
  const query = useAppFiles(appName)
  const [editing, setEditing] = useState<AppFile | 'new' | null>(null)
  const [deleting, setDeleting] = useState<AppFile | null>(null)

  const files = query.data?.files ?? []

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3">
        <CardTitle className="flex items-center gap-2">
          <FileTextGlassIcon className="size-5" aria-hidden="true" />
          {t('title')}
        </CardTitle>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => setEditing('new')}
        >
          <PlusGlassIcon size="1.4em" />
          {t('add')}
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('description')}</p>
        {query.isLoading ? (
          <Skeleton className="h-10 w-full" />
        ) : query.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {t('loadError')}
          </p>
        ) : files.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('empty')}</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {files.map((file) => (
              <li
                key={file.path}
                className="flex items-center justify-between gap-3 px-3 py-2"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span className="truncate font-mono text-sm">
                    {file.path}
                  </span>
                  <Badge variant="outline" className="font-mono">
                    {file.mode}
                  </Badge>
                  {file.redacted ? (
                    <Badge variant="muted">{t('redacted')}</Badge>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setEditing(file)}
                  >
                    <PencilGlassIcon size="1.4em" />
                    <span className="sr-only">
                      {t('edit', { path: file.path })}
                    </span>
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setDeleting(file)}
                  >
                    <TrashGlassIcon size="1.4em" />
                    <span className="sr-only">
                      {t('delete', { path: file.path })}
                    </span>
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      {editing !== null ? (
        <FileDialog
          appName={appName}
          file={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
      {deleting !== null ? (
        <DeleteDialog
          appName={appName}
          file={deleting}
          onClose={() => setDeleting(null)}
        />
      ) : null}
    </Card>
  )
}

function FileDialog({
  appName,
  file,
  onClose,
}: {
  appName: string
  file: AppFile | null
  onClose: () => void
}) {
  const { t } = useTranslation('appFiles')
  const id = useId()
  const put = usePutAppFile(appName)
  const [path, setPath] = useState(file?.path ?? '')
  const [mode, setMode] = useState(file?.mode ?? DEFAULT_MODE)
  // A redacted file has no content to prefill, so editing starts empty.
  const [content, setContent] = useState(file?.content ?? '')
  const [secret, setSecret] = useState(file?.secret ?? false)
  const [restart, setRestart] = useState(false)

  const pathError = path.startsWith('/') ? null : t('validation.path')
  const modeError = MODE_PATTERN.test(mode) ? null : t('validation.mode')
  const invalid = pathError !== null || modeError !== null

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (invalid) return
    put.mutate(
      { path: path.trim(), content, mode, secret, restart },
      {
        onSuccess: () => {
          toast.add({ title: t('toast.saved'), type: 'success' })
          onClose()
        },
        onError: (error) =>
          toast.add({
            title: t('toast.saveFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="space-y-3">
          <DialogHeader>
            <DialogTitle>
              {file ? t('dialog.editTitle') : t('dialog.addTitle')}
            </DialogTitle>
            <DialogDescription>{t('dialog.description')}</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3 sm:flex-row">
            <div className="flex-1 space-y-1">
              <Label htmlFor={`${id}-path`}>{t('dialog.path')}</Label>
              <Input
                id={`${id}-path`}
                value={path}
                onChange={(e) => setPath(e.target.value)}
                disabled={file !== null}
                className="font-mono"
                placeholder={t('dialog.pathPlaceholder')}
                aria-invalid={pathError !== null && path.length > 0}
              />
            </div>
            <div className="space-y-1 sm:w-28">
              <Label htmlFor={`${id}-mode`}>{t('dialog.mode')}</Label>
              <Input
                id={`${id}-mode`}
                value={mode}
                onChange={(e) => setMode(e.target.value)}
                className="font-mono"
                placeholder={t('dialog.modePlaceholder')}
                aria-invalid={modeError !== null}
              />
            </div>
          </div>
          {modeError ? (
            <p className="text-xs text-destructive">{modeError}</p>
          ) : null}
          <div className="space-y-1">
            <Label htmlFor={`${id}-content`}>{t('dialog.content')}</Label>
            <Textarea
              id={`${id}-content`}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={8}
              spellCheck={false}
              className="font-mono"
            />
            {file?.redacted ? (
              <p className="text-xs text-muted-foreground">
                {t('dialog.contentRedactedHint')}
              </p>
            ) : null}
          </div>
          <div className="space-y-2">
            <div className="flex items-start gap-2">
              <Checkbox
                id={`${id}-secret`}
                checked={secret}
                onCheckedChange={(v) => setSecret(v === true)}
              />
              <div className="space-y-0.5">
                <Label htmlFor={`${id}-secret`}>{t('dialog.secret')}</Label>
                <p className="text-xs text-muted-foreground">
                  {t('dialog.secretHelp')}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id={`${id}-restart`}
                checked={restart}
                onCheckedChange={(v) => setRestart(v === true)}
              />
              <Label htmlFor={`${id}-restart`}>{t('dialog.restart')}</Label>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t('dialog.cancel')}
            </Button>
            <Button type="submit" disabled={put.isPending || invalid}>
              {put.isPending ? t('dialog.saving') : t('dialog.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DeleteDialog({
  appName,
  file,
  onClose,
}: {
  appName: string
  file: AppFile
  onClose: () => void
}) {
  const { t } = useTranslation('appFiles')
  const id = useId()
  const del = useDeleteAppFile(appName)
  const [restart, setRestart] = useState(false)

  function confirm() {
    del.mutate(
      { path: file.path, restart },
      {
        onSuccess: () => {
          toast.add({ title: t('toast.deleted'), type: 'success' })
          onClose()
        },
        onError: (error) =>
          toast.add({
            title: t('toast.deleteFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{t('deleteDialog.title')}</DialogTitle>
          <DialogDescription>
            {t('deleteDialog.description', { path: file.path })}
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <Checkbox
            id={`${id}-restart`}
            checked={restart}
            onCheckedChange={(v) => setRestart(v === true)}
          />
          <Label htmlFor={`${id}-restart`}>{t('deleteDialog.restart')}</Label>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('dialog.cancel')}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={del.isPending}
            onClick={confirm}
          >
            {del.isPending
              ? t('deleteDialog.deleting')
              : t('deleteDialog.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

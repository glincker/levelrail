import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { SiteDropZone } from '../dropdeploy/SiteDropZone'
import { useDropPublish } from '../../hooks/useDropPublish'
import { ApiError } from '../../lib/apiError'
import { formatSize } from '../../lib/format'
import { APP_NAME_PATTERN } from '../../lib/newApp'
import { siteNameFrom, type DropSelection } from '../../lib/zipSite'
import { createPagesSite } from '../../queries/pagesSites'
import { suggestSiteName } from './pageHelpers'

type Mode = 'drop' | 'blank'

function blankSelection(name: string): DropSelection {
  const html = `<!doctype html>\n<html lang="en">\n<head><meta charset="utf-8"><title>${name}</title></head>\n<body><h1>${name}</h1></body>\n</html>\n`
  const file = new File([html], 'index.html', { type: 'text/html' })
  return {
    kind: 'folder',
    files: [{ path: 'index.html', file }],
    label: 'index.html',
    bytes: file.size,
    hasIndex: true,
  }
}

/** Compact create flow: name prefilled, then a drop (folder or zip) or a blank starter page. */
export function NewSiteDialog({
  open,
  onOpenChange,
  existingNames,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  existingNames: readonly string[]
}>) {
  const { t } = useTranslation('pagesSites')
  const navigate = useNavigate()
  const { phase, run, reset } = useDropPublish((appName) => {
    onOpenChange(false)
    void navigate({ to: '/pages/$name', params: { name: appName } })
  })
  const [mode, setMode] = useState<Mode>('drop')
  const [selection, setSelection] = useState<DropSelection | null>(null)
  const [typedName, setTypedName] = useState<string | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)

  const name =
    typedName ??
    (selection
      ? siteNameFrom(selection.label) || suggestSiteName(existingNames)
      : suggestSiteName(existingNames))
  const taken = existingNames.includes(name)
  const nameInvalid = !APP_NAME_PATTERN.test(name) || taken
  const busy = creating || phase.state === 'busy'
  const ready = mode === 'blank' || selection !== null
  const failure =
    createError ??
    (phase.state === 'error'
      ? phase.error instanceof ApiError
        ? phase.error.message
        : t('new.failed', { message: '' })
      : null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setCreateError(null)
    setCreating(true)
    try {
      const route = await createPagesSite(name)
      const chosen = mode === 'blank' ? blankSelection(name) : selection
      if (chosen) await run(chosen, name, route === 'fallback')
    } catch (err) {
      setCreateError(
        err instanceof ApiError && err.status === 409
          ? t('new.nameTaken')
          : t('new.failed', {
              message: err instanceof Error ? err.message : '',
            }),
      )
    } finally {
      setCreating(false)
    }
  }

  const handleOpenChange = (next: boolean) => {
    if (busy) return
    if (!next) {
      setSelection(null)
      setTypedName(null)
      setCreateError(null)
      reset()
    }
    onOpenChange(next)
  }

  const stageLabel = phase.state === 'busy' ? t(`new.stage.${phase.stage}`) : ''

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('new.title')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={(e) => void submit(e)} className="space-y-3">
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-40 flex-1 space-y-1.5">
              <Label htmlFor="pages-site-name">{t('new.name')}</Label>
              <Input
                id="pages-site-name"
                value={name}
                disabled={busy}
                aria-invalid={nameInvalid}
                aria-describedby="pages-site-name-hint"
                onChange={(e) => setTypedName(e.target.value.toLowerCase())}
              />
            </div>
            <SegmentedControl
              aria-label={t('new.mode')}
              value={mode}
              disabled={busy}
              onValueChange={(v) => setMode(v === 'blank' ? 'blank' : 'drop')}
              options={[
                { value: 'drop', label: t('new.drop') },
                { value: 'blank', label: t('new.blank') },
              ]}
            />
          </div>
          <p
            id="pages-site-name-hint"
            className="text-xs text-muted-foreground"
          >
            {taken ? t('new.nameTaken') : t('new.nameHint')}
          </p>
          {mode === 'drop' ? (
            selection ? (
              <div className="flex items-center justify-between gap-2 rounded-md border border-border px-3 py-2 text-sm">
                <span className="truncate font-mono">{selection.label}</span>
                <span className="flex items-center gap-2 text-muted-foreground">
                  {formatSize(selection.bytes)}
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => setSelection(null)}
                  >
                    {t('new.change')}
                  </Button>
                </span>
              </div>
            ) : (
              <SiteDropZone onSelect={setSelection} disabled={busy} />
            )
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('new.blankHint')}
            </p>
          )}
          {phase.state === 'busy' ? (
            <Progress
              value={
                phase.stage === 'uploading'
                  ? Math.round(phase.progress * 100)
                  : null
              }
              aria-label={stageLabel}
            >
              <span className="text-xs text-muted-foreground">
                {stageLabel}
              </span>
            </Progress>
          ) : null}
          {failure ? (
            <Alert variant="destructive" role="alert">
              <AlertDescription>{failure}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Link
              to="/apps/new"
              className="text-xs text-muted-foreground underline-offset-2 hover:underline focus-visible:underline"
            >
              {t('new.git')}
            </Link>
            <Button
              type="submit"
              size="sm"
              disabled={busy || nameInvalid || !ready}
            >
              {busy ? t('new.creating') : t('new.create')}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  )
}

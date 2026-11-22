import { lazy, Suspense, useState, type FormEvent } from 'react'
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
import { Skeleton } from '@/components/ui/skeleton'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { SiteDropZone } from '../dropdeploy/SiteDropZone'
import { useDropPublish } from '../../hooks/useDropPublish'
import { ApiError } from '../../lib/apiError'
import { SelectionSummary } from './SelectionSummary'
import { SiteSuccess } from './SiteSuccess'
import { APP_NAME_PATTERN } from '../../lib/newApp'
import { siteNameFrom, type DropSelection } from '../../lib/zipSite'
import { createPagesSite } from '../../queries/pagesSites'
import { suggestSiteName } from './pageHelpers'
import { starterSelection, type StarterId } from './starters'
import { StarterGallery } from './starters/StarterGallery'

// Git mode loads on demand so the pages list keeps its first-load budget.
const GitSiteMode = lazy(() => import('./git/GitSiteMode'))

type Mode = 'drop' | 'blank' | 'git'

/** Compact create flow: name prefilled, then a drop (folder or zip) or a blank starter page. */
export function NewSiteDialog({
  open,
  onOpenChange,
  existingNames,
  initialMode = 'drop',
  initialSelection = null,
}: Readonly<{
  initialMode?: Mode
  initialSelection?: DropSelection | null
  open: boolean
  onOpenChange: (open: boolean) => void
  existingNames: readonly string[]
}>) {
  const { t } = useTranslation('pagesSites')
  const { phase, run, reset } = useDropPublish()
  const [mode, setMode] = useState<Mode>(initialMode)
  const [selection, setSelection] = useState<DropSelection | null>(
    initialSelection,
  )
  const [starter, setStarter] = useState<StarterId>('blank')
  const [typedName, setTypedName] = useState<string | null>(null)
  const [createError, setCreateError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [gitBusy, setGitBusy] = useState(false)
  const [gitRepoName, setGitRepoName] = useState('')

  const name =
    typedName ??
    (mode === 'git' && gitRepoName
      ? gitRepoName
      : selection
        ? siteNameFrom(selection.label) || suggestSiteName(existingNames)
        : suggestSiteName(existingNames))
  const taken = existingNames.includes(name)
  const nameInvalid = !APP_NAME_PATTERN.test(name) || taken
  const busy = creating || gitBusy || phase.state === 'busy'
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
      const chosen =
        mode === 'blank' ? starterSelection(starter, name) : selection
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
        {phase.state === 'done' ? (
          <SiteSuccess name={name} onNavigate={() => handleOpenChange(false)} />
        ) : (
          <form onSubmit={(e) => void submit(e)} className="space-y-3">
            <div className="flex flex-wrap items-end gap-3">
              <div className="min-w-full flex-1 space-y-1.5 min-[420px]:min-w-40">
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
                onValueChange={setMode}
                options={[
                  { value: 'drop', label: t('new.drop') },
                  { value: 'blank', label: t('new.blank') },
                  { value: 'git', label: t('new.git') },
                ]}
              />
            </div>
            <p
              id="pages-site-name-hint"
              className="text-xs text-muted-foreground"
            >
              {taken ? t('new.nameTaken') : t('new.nameHint')}
            </p>
            {mode === 'git' ? (
              <Suspense fallback={<Skeleton className="h-32 w-full" />}>
                <GitSiteMode
                  name={name}
                  nameInvalid={nameInvalid}
                  onRepoName={setGitRepoName}
                  onBusyChange={setGitBusy}
                  onClose={() => handleOpenChange(false)}
                />
              </Suspense>
            ) : mode === 'drop' ? (
              selection ? (
                <div className="flex items-start justify-between gap-2 rounded-md border border-border px-3 py-2">
                  <SelectionSummary selection={selection} />
                  <span className="flex shrink-0 items-center gap-2">
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
              <StarterGallery
                value={starter}
                onChange={setStarter}
                name={name}
                disabled={busy}
              />
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
            {mode === 'git' ? null : (
              <div className="flex justify-end">
                <Button
                  type="submit"
                  size="sm"
                  disabled={busy || nameInvalid || !ready}
                >
                  {busy ? t('new.creating') : t('new.create')}
                </Button>
              </div>
            )}
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}

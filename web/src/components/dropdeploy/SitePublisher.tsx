import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { useDropPublish } from '../../hooks/useDropPublish'
import { ApiError } from '../../lib/apiError'
import { formatSize } from '../../lib/format'
import { siteNameFrom, type DropSelection } from '../../lib/zipSite'
import type { PublishDroppedSiteResult } from '../../queries/dropDeploy'
import { SiteDropZone } from './SiteDropZone'

interface Props {
  /** Existing app to publish to. Omit to create a new static site from the drop. */
  appName?: string
  onPublished?: (appName: string, result: PublishDroppedSiteResult) => void
}

type DropErrorKey =
  | 'drop.errors.tooLarge'
  | 'drop.errors.nameTaken'
  | 'drop.errors.rejected'
  | 'drop.errors.network'
  | 'drop.errors.tooManyFiles'
  | 'drop.errors.generic'

function errorKey(err: unknown): { key: DropErrorKey; detail?: string } {
  if (err instanceof ApiError) {
    if (err.status === 413)
      return { key: 'drop.errors.tooLarge', detail: err.message }
    if (err.status === 409) return { key: 'drop.errors.nameTaken' }
    if (err.status === 400)
      return { key: 'drop.errors.rejected', detail: err.message }
    if (err.status === 0) return { key: 'drop.errors.network' }
    return { key: 'drop.errors.generic', detail: err.message }
  }
  if (err instanceof Error && err.message === 'too-many-files') {
    return { key: 'drop.errors.tooManyFiles' }
  }
  return { key: 'drop.errors.generic' }
}

/** Drop zone, preview and publish button for one static site, new or existing. */
export function SitePublisher({ appName, onPublished }: Props) {
  const { t } = useTranslation('newApp')
  const { phase, run, reset } = useDropPublish(onPublished)
  const [selection, setSelection] = useState<DropSelection | null>(null)
  const [emptyDrop, setEmptyDrop] = useState(false)
  const [typedName, setTypedName] = useState<string | null>(null)
  const isNew = appName === undefined
  const name = isNew
    ? (typedName ?? siteNameFrom(selection?.label ?? ''))
    : appName
  const busy = phase.state === 'busy'
  const nameInvalid = isNew && !/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)

  const clear = () => {
    setSelection(null)
    reset()
  }

  if (!selection) {
    return (
      <div className="space-y-2">
        <SiteDropZone
          onSelect={(s) => {
            setEmptyDrop(s === null)
            setSelection(s)
            reset()
          }}
        />
        {emptyDrop ? (
          <Alert variant="destructive">
            <AlertDescription>{t('drop.empty')}</AlertDescription>
          </Alert>
        ) : null}
      </div>
    )
  }

  const fileCount = selection.kind === 'folder' ? selection.files.length : null
  const stageLabel =
    phase.state === 'busy' ? t(`drop.stage.${phase.stage}`) : ''
  const failure = phase.state === 'error' ? errorKey(phase.error) : null

  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        <dt className="text-muted-foreground">{t('drop.preview.source')}</dt>
        <dd className="font-mono">{selection.label}</dd>
        {fileCount !== null ? (
          <>
            <dt className="text-muted-foreground">{t('drop.preview.files')}</dt>
            <dd>{fileCount}</dd>
          </>
        ) : null}
        <dt className="text-muted-foreground">{t('drop.preview.size')}</dt>
        <dd>{formatSize(selection.bytes)}</dd>
      </dl>
      {selection.kind === 'folder' && !selection.hasIndex ? (
        <Alert>
          <AlertDescription>{t('drop.noIndex')}</AlertDescription>
        </Alert>
      ) : null}
      {isNew ? (
        <div className="space-y-1.5">
          <Label htmlFor="drop-site-name">{t('drop.nameLabel')}</Label>
          <Input
            id="drop-site-name"
            value={name}
            disabled={busy}
            aria-invalid={nameInvalid}
            onChange={(e) => setTypedName(e.target.value.toLowerCase())}
          />
          <p className="text-xs text-muted-foreground">{t('drop.nameHint')}</p>
        </div>
      ) : null}
      {busy ? (
        <Progress
          value={
            phase.stage === 'uploading'
              ? Math.round(phase.progress * 100)
              : null
          }
          aria-label={stageLabel}
        >
          <span className="text-xs text-muted-foreground">{stageLabel}</span>
        </Progress>
      ) : null}
      {failure ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>
            {t(failure.key)}
            {failure.detail ? ` ${failure.detail}` : ''}
          </AlertDescription>
        </Alert>
      ) : null}
      {phase.state === 'done' ? (
        <Alert role="status">
          <AlertDescription>
            {t(
              phase.result.upload.has_index === false
                ? 'drop.doneNoIndex'
                : 'drop.done',
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          disabled={busy || nameInvalid || name === ''}
          onClick={() => void run(selection, name, isNew)}
        >
          {failure
            ? t('drop.retry')
            : isNew
              ? t('drop.publishNew')
              : t('drop.publish')}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={clear}
        >
          {phase.state === 'done' ? t('drop.another') : t('drop.clear')}
        </Button>
      </div>
    </div>
  )
}

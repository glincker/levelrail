import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Progress } from '@/components/ui/progress'
import { useDropPublish } from '../../hooks/useDropPublish'
import { selectionFromDataTransfer } from '../../lib/zipSite'

/** Makes the whole site page a drop target that publishes a new version of the site. */
export function PageDropTarget({
  name,
  children,
}: Readonly<{ name: string; children: ReactNode }>) {
  const { t } = useTranslation('pagesSites')
  const { phase, run } = useDropPublish()
  const [dragging, setDragging] = useState(false)
  const busy = phase.state === 'busy'
  const stageLabel = phase.state === 'busy' ? t(`new.stage.${phase.stage}`) : ''

  return (
    <div
      className="relative space-y-3"
      onDragOver={(e) => {
        if (!busy && e.dataTransfer.types.includes('Files')) {
          e.preventDefault()
          setDragging(true)
        }
      }}
    >
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
      {phase.state === 'error' ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>
            {phase.error instanceof Error
              ? phase.error.message
              : t('domains.addFailed')}
          </AlertDescription>
        </Alert>
      ) : null}
      {phase.state === 'done' ? (
        <Alert role="status">
          <AlertDescription>{t('detail.redeployed')}</AlertDescription>
        </Alert>
      ) : null}
      {children}
      {dragging ? (
        <div
          className="absolute inset-0 z-10 flex items-center justify-center gap-2 rounded-lg border-2 border-dashed border-brand bg-background/90 text-sm font-medium"
          onDragLeave={() => setDragging(false)}
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            void selectionFromDataTransfer(e.dataTransfer).then((sel) => {
              if (sel) void run(sel, name, false)
            })
          }}
        >
          <UploadGlassIcon className="size-5" aria-hidden="true" />
          {t('detail.dropOverlay')}
        </div>
      ) : null}
    </div>
  )
}

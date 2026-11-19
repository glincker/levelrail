import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ExternalLinkGlassIcon,
  ImageGlassIcon,
  RocketGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { DeployPreviewThumb } from '../DeployPreviewThumb'
import { formatAbsoluteTime, formatRelativeTime } from '../../lib/relativeTime'
import { usePreviewHistory, usePreviewStatus } from '../../queries/preview'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { DeployTab } from './DeploySheet'
import { pickPreviewAttempt } from './previewAttempt'

const FRAME = 'aspect-[8/5] w-full rounded-md border border-border bg-muted'

export interface OverviewPreviewProps {
  appName: string
  attempts: DeployAttempt[]
  url: string | null
  onDeploy: (tab: DeployTab) => void
}

function OpenLink({ url, label }: { url: string; label: string }) {
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className={cn(buttonVariants({ variant: 'ghost', size: 'sm' }))}
    >
      <ExternalLinkGlassIcon className="size-5" data-icon="inline-start" />
      {label}
    </a>
  )
}

export default function OverviewPreview({
  appName,
  attempts,
  url,
  onDeploy,
}: OverviewPreviewProps) {
  const { t } = useTranslation('appOverview')
  const status = usePreviewStatus(appName)
  const history = usePreviewHistory(appName)
  const released = pickPreviewAttempt(attempts)

  if (!released) {
    return (
      <section
        aria-label={t('preview.label')}
        className="flex items-center gap-3 rounded-md border border-dashed border-border px-3 py-2 text-sm text-muted-foreground"
      >
        <ImageGlassIcon className="size-5 shrink-0" aria-hidden="true" />
        <p className="min-w-0 flex-1">{t('preview.empty')}</p>
        <Button size="sm" onClick={() => onDeploy('existing-image')}>
          <RocketGlassIcon className="size-5" data-icon="inline-start" />
          {t('preview.deploy')}
        </Button>
      </section>
    )
  }

  const serverOff = status.data !== undefined && !status.data.server_enabled
  const off = status.data?.mode === 'off'
  const canTurnOn =
    status.data !== undefined && !serverOff && status.data.mode !== 'screenshot'
  const record = history.data?.find((r) => r.deployment_id === released.id)
  const capturedAt = record?.status === 'ok' ? record.captured_at : undefined

  return (
    <section aria-label={t('preview.label')} className="max-w-xl space-y-2">
      <div className="flex min-h-8 flex-wrap items-center gap-x-2 gap-y-1">
        <h2 className="text-sm font-medium">{t('preview.label')}</h2>
        {capturedAt ? (
          <time
            dateTime={capturedAt}
            title={formatAbsoluteTime(capturedAt)}
            className="text-xs text-muted-foreground"
          >
            {t('preview.captured', { when: formatRelativeTime(capturedAt) })}
          </time>
        ) : null}
        <div className="ml-auto flex items-center gap-1">
          {canTurnOn ? (
            <Link
              to="/apps/$name/deploy-settings"
              params={{ name: appName }}
              hash="automation"
              className={cn(buttonVariants({ variant: 'ghost', size: 'sm' }))}
            >
              {t('preview.turnOn')}
            </Link>
          ) : null}
          {url ? <OpenLink url={url} label={t('preview.open')} /> : null}
        </div>
      </div>
      {off || serverOff ? (
        <div
          className={cn(
            FRAME,
            'flex flex-col items-center justify-center gap-1 px-4 text-center',
          )}
        >
          <ImageGlassIcon
            className="size-6 text-muted-foreground"
            aria-hidden="true"
          />
          <p className="text-sm text-muted-foreground">
            {serverOff ? t('preview.serverOff') : t('preview.off')}
          </p>
        </div>
      ) : (
        <DeployPreviewThumb
          appName={appName}
          deploymentId={released.id}
          imageUrl={released.preview_image_url}
          commitSha={released.commit_sha?.slice(0, 7)}
          canRecapture
          size="md"
          className="w-full"
        />
      )}
    </section>
  )
}

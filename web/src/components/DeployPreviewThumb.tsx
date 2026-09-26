import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  ImageSquareIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { SkeletonLine, StatusPill } from './kit'
import { previewReasonText } from '../lib/previewReasons'
import { PreviewCard } from './PreviewCard'
import type { PreviewSource } from '../types/preview'
import {
  useCapturePreview,
  usePreviewHistory,
  usePreviewStatus,
  useRefreshWhenCaptureEnds,
} from '../queries/preview'

export interface DeployPreviewThumbProps {
  appName: string
  deploymentId: string
  /** preview_image_url from the deploy history response, if the caller has it. */
  imageUrl?: string
  /** Short commit SHA shown on the text card, when the caller has it. */
  commitSha?: string
  /** Offer a Recapture action; only meaningful for the release now serving. */
  canRecapture?: boolean
  size?: 'sm' | 'md'
  /** Render nothing while this deployment has no preview and previews are off. */
  hideWhenEmpty?: boolean
  className?: string
}

const SOURCE_LABEL: Record<PreviewSource, string> = {
  screenshot: 'Screenshot',
  og_image: 'Site image',
  card: 'Card',
}

const WIDTH: Record<NonNullable<DeployPreviewThumbProps['size']>, string> = {
  sm: 'w-28',
  md: 'w-64',
}

export function DeployPreviewThumb({
  appName,
  deploymentId,
  imageUrl,
  commitSha,
  canRecapture = false,
  size = 'sm',
  hideWhenEmpty = false,
  className,
}: DeployPreviewThumbProps) {
  const history = usePreviewHistory(appName)
  const status = usePreviewStatus(appName)
  const capture = useCapturePreview(appName)
  const [failedSrc, setFailedSrc] = useState<string | null>(null)
  const [loadedSrc, setLoadedSrc] = useState<string | null>(null)
  useRefreshWhenCaptureEnds(appName, status.data?.capturing)

  const record = history.data?.find((r) => r.deployment_id === deploymentId)
  const src = record?.image_url ?? imageUrl
  const canShow = src !== undefined && src !== failedSrc
  const isCard = record?.status === 'ok' && record.source === 'card'
  const sourceLabel =
    record?.status === 'ok' && (canShow || isCard)
      ? SOURCE_LABEL[record.source]
      : undefined
  const previewsOn = status.data?.enabled === true && status.data.server_enabled
  const capturing =
    canRecapture && (status.data?.capturing === true || capture.isPending)

  if (
    !canShow &&
    !record &&
    hideWhenEmpty &&
    !previewsOn &&
    !history.isPending
  ) {
    return null
  }

  function recapture() {
    capture.mutate(undefined, {
      onError: (error) =>
        toast.add({
          title: 'Could not start a preview capture.',
          description: error.message,
          type: 'error',
        }),
    })
  }

  const loading = history.isPending && !canShow
  const imageLoaded = canShow && loadedSrc === src

  return (
    <div
      className={cn('flex shrink-0 flex-col gap-1.5', WIDTH[size], className)}
    >
      <div className="relative aspect-[8/5] overflow-hidden rounded-md border border-border bg-muted">
        {canShow ? (
          <img
            src={src}
            alt={`Preview of deployment ${deploymentId}`}
            loading="lazy"
            decoding="async"
            className="size-full object-cover object-top"
            onLoad={() => setLoadedSrc(src)}
            onError={() => setFailedSrc(src)}
          />
        ) : null}
        {loading || (canShow && !imageLoaded) || capturing ? (
          <div className="absolute inset-0" aria-hidden="true">
            <SkeletonLine className="h-full rounded-none" />
          </div>
        ) : null}
        {isCard && !capturing ? (
          <PreviewCard
            appName={appName}
            meta={record?.meta}
            commitSha={commitSha}
          />
        ) : null}
        {sourceLabel ? (
          <span className="absolute bottom-1 left-1 rounded bg-background/80 px-1.5 py-0.5 text-[10px] font-medium text-foreground">
            {sourceLabel}
          </span>
        ) : null}
        {!canShow && !isCard && !loading && !capturing ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <ImageSquareIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
          </div>
        ) : null}
      </div>
      {!canShow && !isCard && !loading && !capturing && record ? (
        <div className="flex flex-col items-start gap-1">
          <StatusPill tone="neutral" size="sm" label="No preview" />
          <p className="line-clamp-3 text-[11px] leading-snug text-muted-foreground">
            {previewReasonText(record.reason)}
          </p>
        </div>
      ) : null}
      {canRecapture && previewsOn ? (
        <Button
          variant="outline"
          size="sm"
          onClick={recapture}
          disabled={capturing}
        >
          <ArrowsClockwiseIcon
            className={cn('size-3.5', capturing && 'animate-spin')}
            data-icon="inline-start"
          />
          {capturing ? 'Capturing...' : 'Recapture'}
        </Button>
      ) : null}
    </div>
  )
}

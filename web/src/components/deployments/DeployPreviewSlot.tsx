import { useState } from 'react'
import { ImageSquareIcon } from '@phosphor-icons/react/dist/ssr'

export interface DeployPreviewSlotProps {
  url: string | null
  app: string
}

/** Swap point: replace this component's body with the shared preview thumbnail when it lands. */
export function DeployPreviewSlot({ url, app }: DeployPreviewSlotProps) {
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const showImage = url !== null && url !== failedUrl
  return (
    <div className="flex aspect-video w-full items-center justify-center overflow-hidden rounded-lg border border-border bg-muted/40">
      {showImage ? (
        <img
          src={url}
          alt={`Preview of the ${app} deployment`}
          loading="lazy"
          className="size-full object-cover"
          onError={() => {
            setFailedUrl(url)
          }}
        />
      ) : (
        <span className="flex flex-col items-center gap-1 text-xs text-muted-foreground">
          <ImageSquareIcon className="size-6" aria-hidden="true" />
          No preview captured
        </span>
      )}
    </div>
  )
}

import {
  CheckIcon,
  CopyIcon,
  FingerprintIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type { Deployment } from '../../types/deployment'
import { pinnedRef } from '../../lib/deploymentPresentation'
import { shortDigest } from '../../lib/imageDigest'

/** Shows the image by digest. The copy button only ever copies a verified `repo@sha256` ref, never a tag. */
export function ImageRefChip({ d }: { d: Deployment }) {
  const { copied, copy } = useCopyToClipboard()
  const ref = pinnedRef(d)
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <span
          className="inline-flex min-w-0 items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2 py-1 font-mono text-xs"
          title={ref || d.image}
        >
          <FingerprintIcon className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">
            {ref
              ? `${ref.split('@')[0]}@sha256:${shortDigest(d.image_digest)}`
              : d.image}
          </span>
        </span>
        <Button
          variant="outline"
          size="xs"
          disabled={!ref}
          aria-label={
            ref ? 'Copy pinned image reference' : 'No verified digest to copy'
          }
          title={ref ? undefined : 'No verified digest to copy'}
          onClick={() => {
            copy(ref)
          }}
        >
          {copied ? (
            <CheckIcon aria-hidden="true" />
          ) : (
            <CopyIcon aria-hidden="true" />
          )}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      {!ref && (
        <p className="text-xs text-muted-foreground">
          Tag only. The registry digest was not verified for this deploy.
        </p>
      )}
      {d.rollout_state === 'mismatch' && (
        <p className="text-xs text-tone-danger">
          The running container does not match this image.
        </p>
      )}
    </div>
  )
}

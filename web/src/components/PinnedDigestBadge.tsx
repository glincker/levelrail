import {
  CheckIcon,
  CopyIcon,
  FingerprintIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { shortDigest } from '../lib/imageDigest'

// Short pinned digest with a copy button; renders nothing without a digest.
export function PinnedDigestBadge({
  digest,
  className,
}: {
  digest?: string
  className?: string
}) {
  const { t } = useTranslation('common')
  const { copied, copy } = useCopyToClipboard()
  if (!digest) return null
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 align-middle font-mono text-xs',
        className,
      )}
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <span className="inline-flex items-center gap-1 rounded border border-border bg-muted/40 px-1.5 py-0.5">
              <FingerprintIcon className="size-3" aria-hidden="true" />
              {shortDigest(digest)}
            </span>
          }
        />
        <TooltipContent>{t('pinnedDigest.tip')}</TooltipContent>
      </Tooltip>
      <button
        type="button"
        className="inline-flex size-5 items-center justify-center rounded text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        aria-label={t('pinnedDigest.copy')}
        onClick={() => {
          copy(digest)
        }}
      >
        {copied ? (
          <CheckIcon className="size-3" aria-hidden="true" />
        ) : (
          <CopyIcon className="size-3" aria-hidden="true" />
        )}
      </button>
      <span className="sr-only" role="status">
        {copied ? t('pinnedDigest.copied') : ''}
      </span>
    </span>
  )
}

import {
  ArrowSquareOutIcon,
  CheckIcon,
  CopyIcon,
  GlobeIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'

export function UrlLine({
  appName,
  url,
}: {
  appName: string
  url: string | null
}) {
  const { copied, copy } = useCopyToClipboard()
  if (!url) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <GlobeIcon className="size-4" aria-hidden="true" />
        No public URL yet.
        <Link
          to="/apps/$name/domains"
          params={{ name: appName }}
          className="text-primary underline underline-offset-2"
        >
          Add a domain
        </Link>
      </p>
    )
  }
  return (
    <div className="flex min-w-0 items-center gap-1">
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className="inline-flex min-w-0 items-center gap-1.5 truncate text-base font-medium text-primary hover:underline"
      >
        <GlobeIcon className="size-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{url.replace(/^https?:\/\//, '')}</span>
        <ArrowSquareOutIcon className="size-3.5 shrink-0" aria-hidden="true" />
      </a>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={copied ? 'URL copied' : 'Copy URL'}
        onClick={() => copy(url)}
      >
        {copied ? (
          <CheckIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <CopyIcon className="size-3.5" aria-hidden="true" />
        )}
      </Button>
    </div>
  )
}

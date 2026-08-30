import { Link } from '@tanstack/react-router'
import { CheckIcon, CopyIcon, GlobeIcon, CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { certStatusMeta } from '../lib/certStatus'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'

// Shared column grid between the sticky header (routes/domains/index.tsx)
// and every row below, the same "header labels line up with row content
// pixel-for-pixel" reasoning APP_LIST_GRID (AppRow.tsx) already
// establishes for the apps list. The domain column gets its own copy
// button (below), so its cell needs room for both the text and the
// button without the grid itself changing shape.
export const DOMAIN_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,1fr)_8rem_1rem] items-center gap-3'

// This page is deliberately read-mostly: editing a domain (add/remove)
// stays on the owning app's own Domains tab (DomainEditor.tsx), reached
// by clicking through here. Duplicating that add/remove form on a
// cross-app list would mean two places that can desync an app's
// domains, exactly the kind of "second config surface" this codebase's
// architecture doc otherwise argues hard against for ingress state.
export function DomainRow({
  domain,
  cert,
}: {
  domain: Domain
  cert?: CertificateStatus
}) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <Link
      to="/apps/$name/domains"
      params={{ name: domain.service_name }}
      className={`${DOMAIN_LIST_GRID} h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <GlobeIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="flex min-w-0 items-center gap-1">
        <span className="min-w-0 truncate font-mono text-sm font-medium text-foreground">
          {domain.domain}
        </span>
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          className="shrink-0"
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            copy(domain.domain)
          }}
          aria-label={copied ? 'Domain copied' : 'Copy domain'}
        >
          {copied ? <CheckIcon className="size-3.5" /> : <CopyIcon className="size-3.5" />}
        </Button>
      </span>

      <span className="min-w-0 truncate text-xs text-muted-foreground">
        {domain.service_name}
      </span>

      <span className="min-w-0">
        {cert ? (
          <Badge variant={certStatusMeta[cert.status].variant}>
            {certStatusMeta[cert.status].label}
          </Badge>
        ) : (
          <span className="text-xs text-muted-foreground/60 italic">
            no cert yet
          </span>
        )}
      </span>

      <CaretRightIcon
        className="size-4 shrink-0 text-muted-foreground/50"
        aria-hidden="true"
      />
    </Link>
  )
}

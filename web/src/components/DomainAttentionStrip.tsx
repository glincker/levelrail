import { Link } from '@tanstack/react-router'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { certExpiryLabel, certRenewalBadge } from '../lib/certStatus'
import type { CertificateStatus } from '../queries/certificates'
import type { Domain } from '../queries/domains'

const MAX_SHOWN = 5

export interface DomainAttentionEntry {
  domain: Domain
  cert: CertificateStatus
}

function attentionReason(cert: CertificateStatus): string {
  const badge = certRenewalBadge(cert)
  return badge ? badge.label : `Certificate ${certExpiryLabel(cert.not_after)}`
}

// Mirrors PipelineAttentionStrip's visual pattern: an amber banner listing
// the specific rows that need a look, not just a count.
export function DomainAttentionStrip({
  entries,
}: {
  entries: DomainAttentionEntry[]
}) {
  if (entries.length === 0) {
    return null
  }
  const shown = entries.slice(0, MAX_SHOWN)
  const hidden = entries.length - shown.length
  return (
    <section
      aria-label="Needs attention"
      className="rounded-lg border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/30"
    >
      <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <WarningCircleIcon className="size-4" aria-hidden="true" />
        {entries.length === 1
          ? '1 domain needs attention'
          : `${entries.length} domains need attention`}
      </h2>
      <ul className="mt-1 divide-y divide-amber-200 dark:divide-amber-900">
        {shown.map(({ domain, cert }) => (
          <li
            key={domain.domain}
            className="flex flex-wrap items-center gap-2 py-2"
          >
            <Link
              to="/apps/$name/domains"
              params={{ name: domain.service_name }}
              className="min-w-40 flex-1 text-sm text-foreground underline-offset-2 hover:underline"
            >
              <span className="font-mono font-medium">{domain.domain}</span>
              <span className="ml-2 text-xs text-muted-foreground">
                {attentionReason(cert)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {hidden > 0 ? (
        <p className="mt-1 text-xs text-muted-foreground">
          and {hidden} more listed below
        </p>
      ) : null}
    </section>
  )
}

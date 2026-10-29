import { MailGlassIcon } from '@thesvg/ui-react'
import { BrandLogoBadge, BrandTile } from '../BrandLogoBadge'

/** ProviderMark shows a provider's thesvg mark, falling back to a monogram tile (never a blank one). */
export function ProviderMark({
  logoId,
  label,
  generic = false,
}: {
  logoId?: string
  label: string
  /** Protocol rather than a company (SMTP): a generic envelope fits better than a letter. */
  generic?: boolean
}) {
  const fallback = generic ? (
    <MailGlassIcon
      className="size-5 text-muted-foreground"
      aria-hidden="true"
    />
  ) : (
    <BrandTile name={logoId ?? label} label={label} size="xs" />
  )
  if (!logoId) {
    return generic ? (
      <span className="inline-flex size-6 shrink-0 items-center justify-center rounded-md bg-muted ring-1 ring-border">
        {fallback}
      </span>
    ) : (
      <BrandTile name={label} label={label} size="sm" />
    )
  }
  return (
    <BrandLogoBadge
      logoId={logoId}
      className="size-6 p-0.5"
      fallback={fallback}
    />
  )
}

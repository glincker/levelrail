import type { ReactNode } from 'react'
import { TemplateLogo } from './TemplateLogo'

// Neutral light tile so black or white-only marks stay legible in dark
// mode. Renders only the fallback when there is no logo id.
export function BrandLogoBadge({
  logoId,
  fallback,
  className = 'size-7',
}: {
  logoId: string | undefined
  fallback?: ReactNode
  className?: string
}) {
  if (!logoId) return <>{fallback ?? null}</>
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center rounded-md bg-white p-1 ring-1 ring-border ${className}`}
    >
      <TemplateLogo
        id={logoId}
        className="size-full"
        fallback={fallback ?? <span className="size-full" />}
      />
    </span>
  )
}

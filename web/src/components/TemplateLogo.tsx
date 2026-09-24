import { lazy, Suspense, type ReactNode } from 'react'
import { TEMPLATE_LOGO_LOADERS, type LogoComponent } from '../lib/templateLogos'

const LOGOS: Record<string, LogoComponent> = Object.fromEntries(
  Object.entries(TEMPLATE_LOGO_LOADERS).map(([id, loader]) => [
    id,
    lazy(loader) as unknown as LogoComponent,
  ]),
)

export function TemplateLogo({
  id,
  className,
  fallback,
}: {
  id: string
  className?: string
  fallback: ReactNode
}) {
  const Logo = LOGOS[id] as LogoComponent | undefined
  if (!Logo) {
    return <>{fallback}</>
  }
  return (
    <Suspense fallback={fallback}>
      <Logo aria-hidden="true" className={className} />
    </Suspense>
  )
}

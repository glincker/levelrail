import { lazy, Suspense, type ReactNode } from 'react'
import {
  TEMPLATE_LOGO_LOADERS,
  logoLoaderFor,
  type LogoComponent,
  type LogoSurface,
} from '../lib/templateLogos'

function build(surface: LogoSurface): Record<string, LogoComponent> {
  return Object.fromEntries(
    Object.keys(TEMPLATE_LOGO_LOADERS).flatMap((id) => {
      const loader = logoLoaderFor(id, surface)
      return loader ? [[id, lazy(loader)]] : []
    }),
  )
}

const LOGOS: Record<LogoSurface, Record<string, LogoComponent>> = {
  light: build('light'),
  dark: build('dark'),
}

export function TemplateLogo({
  id,
  className,
  fallback,
  surface = 'light',
}: {
  id: string
  className?: string
  fallback: ReactNode
  /** Surface the mark sits on; 'dark' picks the variant registered in DARK_SURFACE_VARIANTS. */
  surface?: LogoSurface
}) {
  const Logo = LOGOS[surface][id] as LogoComponent | undefined
  if (!Logo) {
    return <>{fallback}</>
  }
  return (
    <Suspense fallback={fallback}>
      <Logo aria-hidden="true" className={className} />
    </Suspense>
  )
}

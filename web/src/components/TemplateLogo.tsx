import { lazy, Suspense, type ReactNode } from 'react'
import {
  COMPOSITE_LOGOS,
  TECH_LOGO_LOADERS,
  TEMPLATE_LOGO_LOADERS,
  logoLoaderFor,
  type LogoComponent,
  type LogoSurface,
  type TechLogoKey,
} from '../lib/templateLogos'

function build(surface: LogoSurface): Record<string, LogoComponent> {
  return Object.fromEntries(
    Object.keys(TEMPLATE_LOGO_LOADERS).flatMap((id) => {
      const loader = logoLoaderFor(id, surface)
      return loader ? [[id, lazy(loader)]] : []
    }),
  )
}

const TECH_LOGOS: Record<TechLogoKey, LogoComponent> = {
  nodejs: lazy(TECH_LOGO_LOADERS.nodejs),
  postgresql: lazy(TECH_LOGO_LOADERS.postgresql),
  redis: lazy(TECH_LOGO_LOADERS.redis),
  nextjs: lazy(TECH_LOGO_LOADERS.nextjs),
  fastapi: lazy(TECH_LOGO_LOADERS.fastapi),
  nginx: lazy(TECH_LOGO_LOADERS.nginx),
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
  const composite = COMPOSITE_LOGOS[id]
  if (composite) {
    const [primary, secondary] = composite
    const Primary = TECH_LOGOS[primary]
    const Secondary = secondary ? TECH_LOGOS[secondary] : undefined
    return (
      <span className="relative inline-flex">
        <Suspense fallback={fallback}>
          <Primary aria-hidden="true" className={className} />
          {Secondary ? (
            <span
              data-slot="composite-badge"
              className="absolute -right-2 -bottom-2 flex size-[48%] items-center justify-center rounded-md bg-white p-0.5 ring-1 ring-border"
            >
              <Secondary aria-hidden="true" className="size-full" />
            </span>
          ) : null}
        </Suspense>
      </span>
    )
  }
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

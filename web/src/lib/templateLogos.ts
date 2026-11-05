import { createElement, type ComponentType, type SVGProps } from 'react'
import {
  MISSING_TEMPLATE_LOGOS,
  TEMPLATE_LOGO_LOADERS,
  type LogoComponent,
} from './templateLogos.generated'

export { TEMPLATE_LOGO_LOADERS, type LogoComponent }

export type LogoSurface = 'light' | 'dark'
type LogoVariant = 'light' | 'dark' | 'mono'
type VariantLogo = ComponentType<
  SVGProps<SVGSVGElement> & { variant?: LogoVariant }
>

// Marks whose default variant is white-only and vanishes on the white
// BrandTile; they render their 'light' (black) variant there instead.
const LIGHT_SURFACE_VARIANTS: Record<string, LogoVariant> = {
  ollama: 'light',
  'ollama-deepseek': 'light',
  'ollama-llama3': 'light',
  'ollama-mistral': 'light',
  'ollama-phi': 'light',
  'ollama-qwen': 'light',
  axiom: 'light',
  flask: 'light',
  mattermost: 'light',
  qdrant: 'light',
  rust: 'light',
  browserless: 'light',
  resend: 'light',
}

// Dark-surface hook: add an id here to pick a thesvg variant when its tile
// is rendered with surface="dark". The tile stays white until a caller opts in.
export const DARK_SURFACE_VARIANTS: Record<string, LogoVariant> = {
  ollama: 'dark',
  'ollama-deepseek': 'dark',
  'ollama-llama3': 'dark',
  'ollama-mistral': 'dark',
  'ollama-phi': 'dark',
  'ollama-qwen': 'dark',
  axiom: 'dark',
  flask: 'dark',
  mattermost: 'dark',
  qdrant: 'dark',
  rust: 'dark',
  browserless: 'dark',
}

export function logoLoaderFor(
  id: string,
  surface: LogoSurface = 'light',
): (() => Promise<{ default: LogoComponent }>) | undefined {
  const base = TEMPLATE_LOGO_LOADERS[id]
  if (!base) return undefined
  const variant =
    surface === 'dark' ? DARK_SURFACE_VARIANTS[id] : LIGHT_SURFACE_VARIANTS[id]
  if (!variant) return base
  return () =>
    base().then((m) => ({
      default: (props: SVGProps<SVGSVGElement>) =>
        createElement(m.default as VariantLogo, { ...props, variant }),
    }))
}

export const TECH_LOGO_LOADERS = {
  nodejs: () => import('@thesvg/react/nodejs'),
  postgresql: () => import('@thesvg/react/postgresql'),
  redis: () => import('@thesvg/react/redis'),
  nextjs: () => import('@thesvg/react/nextjs'),
  fastapi: () => import('@thesvg/react/fastapi'),
  nginx: () => import('@thesvg/react/nginx'),
} satisfies Record<string, () => Promise<{ default: LogoComponent }>>

export type TechLogoKey = keyof typeof TECH_LOGO_LOADERS

// Starter kits compose two products: [primary, optional badge] by technology.
export const COMPOSITE_LOGOS: Record<string, [TechLogoKey, TechLogoKey?]> = {
  'node-postgres-starter': ['nodejs', 'postgresql'],
  'redis-cache-starter': ['redis', 'nodejs'],
  'nextjs-postgres-starter': ['nextjs', 'postgresql'],
  'static-site-api-starter': ['nginx', 'nodejs'],
  'fastapi-postgres-starter': ['fastapi', 'postgresql'],
  'worker-redis-queue-starter': ['nodejs', 'redis'],
  'nginx-multi-backend-starter': ['nginx', 'nodejs'],
}
// No thesvg mark exists for these products yet; they render the monogram tile.
// Generated from the catalog; composites are drawn from tech marks instead.
export const KNOWN_MISSING_LOGOS: readonly string[] =
  MISSING_TEMPLATE_LOGOS.filter((id) => !(id in COMPOSITE_LOGOS))

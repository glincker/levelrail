import type { ReactNode } from 'react'
import {
  CloudCheckGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  KeyGlassIcon,
  LockGlassIcon,
  BellGlassIcon,
  MailGlassIcon,
  ServerGlassIcon,
  PackageGlassIcon,
  WebhookGlassIcon,
} from '@thesvg/ui-react'
import { BrandIcon } from '../BrandIcon'
import {
  integrationGlyph,
  type IntegrationUiIcon,
} from '../../lib/integrationBrands'
import type { ConnectionKind } from '../../types/connections'

const KIND_ICON: Record<ConnectionKind, ReactNode> = {
  git: <GitBranchGlassIcon className="size-5" aria-hidden="true" />,
  registry: <PackageGlassIcon className="size-5" aria-hidden="true" />,
  notify: <BellGlassIcon className="size-5" aria-hidden="true" />,
  secret: <LockGlassIcon className="size-5" aria-hidden="true" />,
  dns: <GlobeGlassIcon className="size-5" aria-hidden="true" />,
  cloud: <CloudCheckGlassIcon className="size-5" aria-hidden="true" />,
  webhook: <WebhookGlassIcon className="size-5" aria-hidden="true" />,
}

const UI_ICON: Record<IntegrationUiIcon, ReactNode> = {
  mail: <MailGlassIcon className="size-5" aria-hidden="true" />,
  bell: <BellGlassIcon className="size-5" aria-hidden="true" />,
  webhook: <WebhookGlassIcon className="size-5" aria-hidden="true" />,
  server: <ServerGlassIcon className="size-5" aria-hidden="true" />,
}

function kindIcon(kind: string): ReactNode {
  return (
    KIND_ICON[kind as ConnectionKind] ?? (
      <KeyGlassIcon className="size-5" aria-hidden="true" />
    )
  )
}

// Brand mark, semantic UI icon or kind glyph; ObjectCard puts it in a tile.
// Brand marks sit on a light chip so dark-on-dark marks stay legible in dark mode.
export function ConnectionGlyph({
  provider,
  kind,
}: Readonly<{ provider: string; kind: string }>) {
  const spec = integrationGlyph(kind, provider)
  if (spec?.brand) {
    return (
      <span className="inline-flex size-8 items-center justify-center rounded-md bg-white p-1 text-neutral-900 ring-1 ring-border">
        <BrandIcon name={spec.brand} className="size-full" />
      </span>
    )
  }
  return spec?.ui ? UI_ICON[spec.ui] : kindIcon(kind)
}

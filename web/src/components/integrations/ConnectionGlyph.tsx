import type { ReactNode } from 'react'
import {
  CloudCheckGlassIcon,
  GitBranchGlassIcon,
  GlobeGlassIcon,
  KeyGlassIcon,
  LockGlassIcon,
  BellGlassIcon,
  PackageGlassIcon,
  WebhookGlassIcon,
} from '@thesvg/ui-react'
import { BrandIcon, isBrandIconName } from '../BrandIcon'
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

function kindIcon(kind: string): ReactNode {
  return (
    KIND_ICON[kind as ConnectionKind] ?? (
      <KeyGlassIcon className="size-5" aria-hidden="true" />
    )
  )
}

// Brand mark when the provider has one, else the kind glyph, in a fixed tile.
export function ConnectionGlyph({
  provider,
  kind,
}: Readonly<{ provider: string; kind: string }>) {
  const brand = provider.toLowerCase()
  return (
    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background text-muted-foreground">
      {isBrandIconName(brand) ? (
        <BrandIcon name={brand} className="size-5" />
      ) : (
        kindIcon(kind)
      )}
    </span>
  )
}

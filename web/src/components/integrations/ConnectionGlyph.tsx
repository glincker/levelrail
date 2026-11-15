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

// Brand mark when the provider has one, else the kind glyph; ObjectCard puts it in a tile.
export function ConnectionGlyph({
  provider,
  kind,
}: Readonly<{ provider: string; kind: string }>) {
  const brand = provider.toLowerCase()
  return isBrandIconName(brand) ? (
    <BrandIcon name={brand} className="size-5" />
  ) : (
    kindIcon(kind)
  )
}

import type { ComponentType, SVGProps } from 'react'
import {
  BlockStorageGlassIcon,
  FolderGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { backupTargetBrand } from '../lib/providerBrands'
import type { BackupProvider } from '../types/backupTarget'
import { BrandTile, type BrandTileSize } from './BrandLogoBadge'

type Glyph = ComponentType<SVGProps<SVGSVGElement>>

// Providers with no brand mark get a neutral glyph, never a bare initial.
const PROVIDER_GLYPH: Partial<Record<BackupProvider, Glyph>> = {
  custom: BlockStorageGlassIcon,
  sftp: TerminalGlassIcon,
  local: FolderGlassIcon,
}

export function BackupProviderTile({
  provider,
  endpoint,
  label,
  size = 'xs',
}: Readonly<{
  provider: BackupProvider
  endpoint?: string
  label: string
  size?: BrandTileSize
}>) {
  const name = backupTargetBrand(provider, endpoint)
  const Glyph = PROVIDER_GLYPH[provider]
  return (
    <BrandTile name={name} label={label} size={size}>
      {!name && Glyph ? (
        <Glyph className="size-3.5" aria-hidden="true" />
      ) : undefined}
    </BrandTile>
  )
}

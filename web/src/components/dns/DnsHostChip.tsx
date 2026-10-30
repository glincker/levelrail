import { GlobeGlassIcon } from '@thesvg/ui-react'
import { BrandTile } from '@/components/BrandLogoBadge'
import { isBrandIconName } from '@/components/BrandIcon'
import { brandKeyFor, providerLabel } from './dnsSetup'

/** Provider logo tile; a neutral globe when the mark does not exist. Never loads a third-party image. */
export function DnsHostTile({
  provider,
  size = 'sm',
}: {
  provider: string
  size?: 'xs' | 'sm' | 'md'
}) {
  const key = brandKeyFor(provider)
  const label = providerLabel(provider) || provider
  if (!provider || !isBrandIconName(key)) {
    return (
      <BrandTile name="" label={label || '?'} size={size}>
        <GlobeGlassIcon size="1.2em" />
      </BrandTile>
    )
  }
  return <BrandTile name={key} label={label} size={size} />
}

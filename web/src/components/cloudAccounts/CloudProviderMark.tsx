import { CloudGlassIcon } from '@thesvg/ui-react'
import { BrandIcon, isBrandIconName } from '../BrandIcon'
import { nodeProviderBrand } from '../../lib/providerBrands'

// Brand mark when thesvg has one, the generic cloud glyph otherwise.
export function CloudProviderMark({
  provider,
  className = 'size-5',
}: Readonly<{ provider: string; className?: string }>) {
  const brand = nodeProviderBrand(provider)
  if (brand !== '' && isBrandIconName(brand)) {
    return <BrandIcon name={brand} className={className} />
  }
  return <CloudGlassIcon className={className} aria-hidden="true" />
}

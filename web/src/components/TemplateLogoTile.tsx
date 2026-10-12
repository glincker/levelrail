import { BrandTile, type BrandTileSize } from './BrandLogoBadge'
import { TemplateLogo } from './TemplateLogo'

const MARK_SIZE: Record<BrandTileSize, string> = {
  xs: 'size-3.5',
  sm: 'size-5',
  md: 'size-6',
  lg: 'size-7',
}

/** TemplateLogoTile shows a catalog template's thesvg mark in a BrandTile, with the name initial as fallback. */
export function TemplateLogoTile({
  id,
  name,
  size = 'sm',
}: {
  id: string
  name: string
  size?: BrandTileSize
}) {
  const initial = (
    <span className="font-mono text-sm font-medium uppercase">
      {name.slice(0, 1)}
    </span>
  )
  return (
    <BrandTile name="" label={name} size={size}>
      <TemplateLogo id={id} className={MARK_SIZE[size]} fallback={initial} />
    </BrandTile>
  )
}

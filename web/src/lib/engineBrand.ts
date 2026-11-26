import { isBrandIconName, type BrandIconName } from '@/components/BrandIcon'

// Engine ids the registry may send that differ from the brand name.
const ENGINE_ALIASES: Record<string, BrandIconName> = {
  postgresql: 'postgres',
  mongo: 'mongodb',
  'aws-dynamodb': 'dynamodb',
  rds: 'aws-rds',
}

/** Brand mark for a database engine or hosted service, or null when thesvg
 *  has none (callers then show a neutral glyph with the engine initials). */
export function engineBrand(engine: string): BrandIconName | null {
  const id = engine.trim().toLowerCase()
  const alias = ENGINE_ALIASES[id]
  if (alias) return alias
  return isBrandIconName(id) ? id : null
}

/** Two-letter initials for the fallback tile. */
export function engineInitials(engine: string): string {
  return engine
    .replaceAll(/[^a-z0-9]/gi, '')
    .slice(0, 2)
    .toUpperCase()
}

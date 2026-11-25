import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { BrandIcon } from '@/components/BrandIcon'
import { engineBrand, engineInitials } from '@/lib/engineBrand'

// One engine logo for every surface: the thesvg mark, else a neutral
// database glyph with the engine initials so nothing renders blank.
export function EngineMark({
  engine,
  className = 'size-5',
}: Readonly<{ engine: string; className?: string }>) {
  const brand = engineBrand(engine)
  if (brand) return <BrandIcon name={brand} className={className} />
  return (
    <span className="relative inline-flex shrink-0 items-center justify-center text-muted-foreground">
      <DatabaseGlassIcon className={className} aria-hidden="true" />
      <span
        aria-hidden="true"
        className="absolute text-[0.5rem] leading-none font-semibold"
      >
        {engineInitials(engine)}
      </span>
    </span>
  )
}

import { BrandIcon, isBrandIconName } from '@/components/BrandIcon'
import { engineInitials } from '@/lib/engineBrand'
import { cn } from '@/lib/utils'

// Brand mark from the preset's brand id, else initials on a neutral tile.
export function ProviderMark({
  brand,
  label,
  className = 'size-6',
}: Readonly<{ brand: string; label: string; className?: string }>) {
  if (isBrandIconName(brand)) {
    return <BrandIcon name={brand} className={className} />
  }
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-md bg-muted text-[0.6rem] font-semibold text-muted-foreground',
        className,
      )}
    >
      {engineInitials(label)}
    </span>
  )
}

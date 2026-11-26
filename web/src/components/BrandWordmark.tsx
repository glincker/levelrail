import { useBrand } from '../hooks/useBrand'
import { cn } from '../lib/utils'

// Light and dark variants swap through the theme class, no JS needed.
export function BrandWordmark({
  className,
  width = 100,
  height = 35,
}: {
  className?: string
  width?: number
  height?: number
}) {
  const brand = useBrand()
  return (
    <>
      <img
        src="/levelrail-wordmark-light.svg"
        alt={brand.Name}
        width={width}
        height={height}
        className={cn('block dark:hidden', className)}
      />
      <img
        src="/levelrail-wordmark-dark.svg"
        alt=""
        aria-hidden="true"
        width={width}
        height={height}
        className={cn('hidden dark:block', className)}
      />
    </>
  )
}

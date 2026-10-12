import { useMemo } from 'react'
import { useBrand } from '../hooks/useBrand'
import { sanitizeSvg } from '../lib/sanitizeSvg'

// Shared by every brand avatar chip (sidebar, login, forgot/reset
// password, accept invite): the real mark when brand.yaml provides one,
// else the brand's first letter. Callers own their own container's size
// and background; this only picks the glyph content, so the SVG wrapper
// defaults to filling that container exactly.
export function BrandMarkGlyph({
  svgWrapperClassName = 'flex size-full items-center justify-center [&_svg]:size-full',
}: {
  svgWrapperClassName?: string
}) {
  const brand = useBrand()
  const logoSvg = useMemo(() => sanitizeSvg(brand.LogoSVG), [brand.LogoSVG])
  if (logoSvg) {
    return (
      <span
        className={svgWrapperClassName}
        dangerouslySetInnerHTML={{ __html: logoSvg }}
      />
    )
  }
  return <>{(brand.ShortName || brand.Name || 'L').charAt(0).toUpperCase()}</>
}

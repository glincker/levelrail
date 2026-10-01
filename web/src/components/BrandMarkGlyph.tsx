import { useBrand } from '../hooks/useBrand'

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
  if (brand.LogoSVG) {
    return (
      <span
        className={svgWrapperClassName}
        dangerouslySetInnerHTML={{ __html: brand.LogoSVG }}
      />
    )
  }
  return <>{(brand.ShortName || brand.Name || 'L').charAt(0).toUpperCase()}</>
}

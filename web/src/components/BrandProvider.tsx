import { useEffect, type ReactNode } from 'react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { brandQueryOptions } from '../queries/brand'
import { BrandContext } from '../lib/brandContext'

// Rebrandability rule: the frontend reads brand from a /api/v1/brand
// endpoint on boot and hydrates a React context, so no product name or
// styling is hardcoded in components. routes/__root.tsx used to carry a
// comment flagging this as deferred work; this closes it. See
// hooks/useBrand.ts for the consumer side.
//
// Relies on the root route's loader having already primed
// brandQueryOptions() via queryClient.ensureQueryData (see
// routes/__root.tsx), the same "loader fills the cache, the component
// only reads it" split every other useSuspenseQuery call site in this
// app already follows (AppListPage, AppDetailPage). Because the data is
// guaranteed present by the time this renders, this never actually
// suspends, so no <Suspense> boundary is needed here, matching the rest
// of the app.
export function BrandProvider({ children }: { children: ReactNode }) {
  const { data: brand } = useSuspenseQuery(brandQueryOptions())

  // --brand-accent/-dark were dead CSS variables until this effect: the
  // Brand type carried PrimaryColor over the wire but nothing applied it.
  // Set here (not baked into index.css) so brand.yaml stays the one
  // source of truth adr/024 calls for, no rebuild required to re-skin.
  useEffect(() => {
    const root = document.documentElement.style
    root.setProperty('--brand-accent', brand.PrimaryColor)
    root.setProperty(
      '--brand-accent-dark',
      brand.PrimaryColorDark ?? brand.PrimaryColor,
    )
  }, [brand.PrimaryColor, brand.PrimaryColorDark])

  return <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>
}

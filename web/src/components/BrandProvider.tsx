import type { ReactNode } from 'react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { brandQueryOptions } from '../queries/brand'
import { BrandContext } from '../lib/brandContext'

// CLAUDE.md section 3: "Frontend reads brand from a /api/v1/brand
// endpoint on boot, hydrates a React context. No hardcoded strings in
// components." routes/__root.tsx used to carry a comment flagging this
// as deferred work; this closes it (docs-local/research/dashboard-gap-
// audit-and-devmode.md gap #6). See hooks/useBrand.ts for the consumer
// side.
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
  return <BrandContext.Provider value={brand}>{children}</BrandContext.Provider>
}

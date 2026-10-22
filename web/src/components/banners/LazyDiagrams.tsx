import { lazy, Suspense } from 'react'

const AppsDiagram = lazy(() => import('./AppsDiagram'))
const DomainsDiagram = lazy(() => import('./DomainsDiagram'))

type Size = 'hero' | 'compact'

export function LazyAppsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <AppsDiagram size={size} />
    </Suspense>
  )
}

export function LazyDomainsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <DomainsDiagram size={size} />
    </Suspense>
  )
}

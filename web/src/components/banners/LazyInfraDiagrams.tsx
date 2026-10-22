import { lazy, Suspense } from 'react'

const NodesDiagram = lazy(() => import('./NodesDiagram'))
const HealthDiagram = lazy(() => import('./HealthDiagram'))
const InfrastructureDiagram = lazy(() => import('./InfrastructureDiagram'))

type Size = 'hero' | 'compact'

export function LazyNodesDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <NodesDiagram size={size} />
    </Suspense>
  )
}

export function LazyHealthDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <HealthDiagram size={size} />
    </Suspense>
  )
}

export function LazyInfrastructureDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <InfrastructureDiagram size={size} />
    </Suspense>
  )
}

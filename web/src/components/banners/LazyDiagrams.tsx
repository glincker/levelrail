import { lazy, Suspense } from 'react'

const AppsDiagram = lazy(() => import('./AppsDiagram'))
const DomainsDiagram = lazy(() => import('./DomainsDiagram'))
const BuildsDiagram = lazy(() => import('./BuildsDiagram'))
const DeploymentsDiagram = lazy(() => import('./DeploymentsDiagram'))
const BackupsDiagram = lazy(() => import('./BackupsDiagram'))
const LoadBalancersDiagram = lazy(() => import('./LoadBalancersDiagram'))
const ProjectsDiagram = lazy(() => import('./ProjectsDiagram'))

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

export function LazyBuildsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <BuildsDiagram size={size} />
    </Suspense>
  )
}

export function LazyDeploymentsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <DeploymentsDiagram size={size} />
    </Suspense>
  )
}

export function LazyBackupsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <BackupsDiagram size={size} />
    </Suspense>
  )
}

export function LazyLoadBalancersDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <LoadBalancersDiagram size={size} />
    </Suspense>
  )
}

export function LazyProjectsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <ProjectsDiagram size={size} />
    </Suspense>
  )
}

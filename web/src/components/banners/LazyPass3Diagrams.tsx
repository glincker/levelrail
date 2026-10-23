import { lazy, Suspense } from 'react'

type Size = 'hero' | 'compact'

const ImagesDiagram = lazy(() => import('./ImagesDiagram'))
const RegistryDiagram = lazy(() => import('./RegistryDiagram'))
const DnsDiagram = lazy(() => import('./DnsDiagram'))
const PrivateNetworksDiagram = lazy(() => import('./PrivateNetworksDiagram'))
const FirewallDiagram = lazy(() => import('./FirewallDiagram'))
const CdnDiagram = lazy(() => import('./CdnDiagram'))
const TopologyDiagram = lazy(() => import('./TopologyDiagram'))
const CloudProvidersDiagram = lazy(() => import('./CloudProvidersDiagram'))
const StatusDiagram = lazy(() => import('./StatusDiagram'))

export function LazyImagesDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <ImagesDiagram size={size} />
    </Suspense>
  )
}

export function LazyRegistryDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <RegistryDiagram size={size} />
    </Suspense>
  )
}

export function LazyDnsDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <DnsDiagram size={size} />
    </Suspense>
  )
}

export function LazyPrivateNetworksDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <PrivateNetworksDiagram size={size} />
    </Suspense>
  )
}

export function LazyFirewallDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <FirewallDiagram size={size} />
    </Suspense>
  )
}

export function LazyCdnDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <CdnDiagram size={size} />
    </Suspense>
  )
}

export function LazyTopologyDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <TopologyDiagram size={size} />
    </Suspense>
  )
}

export function LazyCloudProvidersDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <CloudProvidersDiagram size={size} />
    </Suspense>
  )
}

export function LazyStatusDiagram({ size }: { size: Size }) {
  return (
    <Suspense fallback={null}>
      <StatusDiagram size={size} />
    </Suspense>
  )
}

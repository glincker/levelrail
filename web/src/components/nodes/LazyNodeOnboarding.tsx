import { lazy, Suspense } from 'react'
import { Skeleton } from '@/components/ui/skeleton'

const NodeDomainsCard = lazy(() => import('./NodeDomainsCard'))
const NodeOnboardingPanel = lazy(() =>
  import('./NodeOnboardingPanel').then((m) => ({
    default: m.NodeOnboardingPanel,
  })),
)

export function LazyNodeDomainsCard() {
  return (
    <Suspense fallback={<Skeleton className="h-28 w-full" />}>
      <NodeDomainsCard />
    </Suspense>
  )
}

export function LazyNodeOnboardingPanel({ nodeName }: { nodeName: string }) {
  return (
    <Suspense fallback={<Skeleton className="h-24 w-full" />}>
      <NodeOnboardingPanel nodeName={nodeName} />
    </Suspense>
  )
}

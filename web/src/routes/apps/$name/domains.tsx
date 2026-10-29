import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { useApp } from '../../../queries/apps'
import { certificatesQueryOptions } from '../../../queries/certificates'
import { AppMaintenanceToggle } from '../../../components/maintenance/AppMaintenanceToggle'
import { DomainEditor } from '../../../components/DomainEditor'
import { AppIngressLimitsCard } from '../../../components/AppIngressLimitsCard'
import { ShieldRecommendedCard } from '../../../components/ShieldRecommendedCard'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

// Former "domains" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed;
// primes certificates here for DomainEditor's inline TLS status.
export const Route = createFileRoute('/apps/$name/domains')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(certificatesQueryOptions()),
  component: DomainsSection,
  pendingComponent: DomainsSectionSkeleton,
})

function DomainsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-4">
      <AppMaintenanceToggle appName={name} />
      <DomainEditor app={app} />
      <ShieldRecommendedCard appName={name} />
      <AppIngressLimitsCard appName={name} />
    </div>
  )
}

// Mirrors DomainEditor's own card: header, a couple of domain rows (input
// plus a few status lines), then the add/save button row.
function DomainsSectionSkeleton() {
  const { t } = useTranslation('domains')
  return (
    <Card aria-hidden="true">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeGlassIcon className="size-4.5" />
          {t('editor.title')}
        </CardTitle>
        <Skeleton className="h-4 w-72" />
      </CardHeader>
      <CardContent className="space-y-4">
        {Array.from({ length: 2 }, (_, i) => (
          <div key={i} className="space-y-2">
            <Skeleton className="h-9 w-full rounded-md" />
            <Skeleton className="h-3 w-48" />
            <Skeleton className="h-3 w-36" />
          </div>
        ))}
        <div className="flex items-center gap-2 pt-1">
          <Skeleton className="h-8 w-28 rounded-md" />
          <Skeleton className="h-8 w-28 rounded-md" />
        </div>
      </CardContent>
    </Card>
  )
}

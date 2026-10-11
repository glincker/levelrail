import { createFileRoute } from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { WarningCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { routeErrorMessage } from '@/lib/apiError'
import { ZoneDetail } from '../../components/dns/ZoneDetail'
import { dnsRecordsQueryOptions, dnsZoneQueryOptions } from '../../queries/dns'

export const Route = createFileRoute('/dns/$zone')({
  loader: ({ context: { queryClient }, params: { zone } }) =>
    Promise.all([
      queryClient.ensureQueryData(dnsZoneQueryOptions(zone)),
      queryClient.ensureQueryData(dnsRecordsQueryOptions(zone)),
    ]),
  component: ZoneRoute,
  pendingComponent: () => <TableSkeleton columnCount={6} rowCount={8} />,
  errorComponent: ZoneError,
})

function ZoneRoute() {
  const { zone } = Route.useParams()
  return <ZoneDetail zoneRef={zone} />
}

function ZoneError({ error }: ErrorComponentProps) {
  const { t } = useTranslation('dns')
  return (
    <EmptyState
      icon={<WarningCircleIcon className="size-5" />}
      title={t('zone.loadFailed')}
      description={routeErrorMessage(error)}
    />
  )
}

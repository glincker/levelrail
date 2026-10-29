import { createFileRoute } from '@tanstack/react-router'
import type { ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { routeErrorMessage } from '@/lib/apiError'
import { ZonesPage } from '../../components/dns/ZonesPage'
import { dnsZonesQueryOptions } from '../../queries/dns'

export const Route = createFileRoute('/dns/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(dnsZonesQueryOptions()),
  component: ZonesPage,
  pendingComponent: () => <TableSkeleton columnCount={4} rowCount={5} />,
  errorComponent: DnsError,
})

function DnsError({ error }: ErrorComponentProps) {
  const { t } = useTranslation('dns')
  return (
    <EmptyState
      icon={<CircleAlertGlassIcon className="size-6 text-tone-danger" />}
      title={t('page.loadFailed')}
      description={routeErrorMessage(error)}
    />
  )
}

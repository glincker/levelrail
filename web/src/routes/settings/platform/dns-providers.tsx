import { createFileRoute } from '@tanstack/react-router'
import { cloudflareDnsSettingsQueryOptions } from '../../../queries/cloudflareDns'
import { route53DnsSettingsQueryOptions } from '../../../queries/route53Dns'
import { DnsProvidersPage } from '../../../components/platform/DnsProvidersPage'

export const Route = createFileRoute('/settings/platform/dns-providers')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(cloudflareDnsSettingsQueryOptions()),
      queryClient.ensureQueryData(route53DnsSettingsQueryOptions()),
    ]),
  component: DnsProvidersPage,
})

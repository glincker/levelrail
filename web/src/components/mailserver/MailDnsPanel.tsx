import { DomainDnsSetup } from '@/components/dns/DomainDnsSetup'
import type { MailCheckReport } from '@/queries/mailDns'
import type { MailStatus } from '@/queries/mailStatus'

/** MailDnsPanel hosts the shared DNS wizard for the mail domain. */
export function MailDnsPanel({
  onReport,
  onDomain,
  defaults,
  previewToken = 0,
}: {
  report?: MailCheckReport | undefined
  onReport: (r: MailCheckReport) => void
  onDomain?: (domain: string) => void
  defaults?: Pick<MailStatus, 'domain' | 'hostname'>
  previewToken?: number
}) {
  return (
    <DomainDnsSetup
      kind="mail"
      defaults={defaults}
      onReport={onReport}
      onDomain={onDomain}
      previewToken={previewToken}
    />
  )
}

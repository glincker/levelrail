import type { DomainCheckPanelData } from '../components/DomainDnsCheck'

export type DnsRecordType = 'A' | 'AAAA' | 'CNAME'

function dnsRecordType(host: string): DnsRecordType {
  if (host.includes(':')) return 'AAAA'
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return 'A'
  return 'CNAME'
}

export interface DnsRecordOption {
  type: DnsRecordType
  value: string
}

// One option per record type the operator could actually add. A raw IP
// APP_PUBLIC_HOST has exactly one: its own family, nothing to choose
// between. A hostname APP_PUBLIC_HOST offers CNAME first (the simplest,
// works even if the server's IP changes later) plus a direct A/AAAA
// alternative for each family it resolves to, since some registrars
// reject CNAME at the domain apex.
export function dnsRecordOptions(
  data: DomainCheckPanelData,
): DnsRecordOption[] {
  const host = data.expected_host
  if (!host) return []
  const type = dnsRecordType(host)
  if (type !== 'CNAME') return [{ type, value: host }]
  const options: DnsRecordOption[] = [{ type: 'CNAME', value: host }]
  if (data.expected_ipv4?.[0])
    options.push({ type: 'A', value: data.expected_ipv4[0] })
  if (data.expected_ipv6?.[0])
    options.push({ type: 'AAAA', value: data.expected_ipv6[0] })
  return options
}
